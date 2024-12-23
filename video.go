package main

import (
	"fmt"
	"sync"
	"time"

	"gocv.io/x/gocv"
)

const (
	// frames per second for the cameras
	DefaultFPS       = 120.0
	DefaultCamWidth  = 1280
	DefaultCamHeight = 720

	// default playback speed at half speed
	DefaultPlaybackSpeed = 0.25
	// records 3 seconds before and after impact
	DefaultSecondsToRecord = 4
	// duration of video to capture after event
	DefaultDurationToCaptureAfterEvent = DefaultSecondsToRecord * time.Second / 2
)

type VideoProfileEnum string

// Manages 2 video streams to capture both front & side profile.
type VideoProfiles struct {
	front *VideoProfile
	back  *VideoProfile
}

func NewVideoProfiles() (*VideoProfiles, error) {
	front, err := NewVideoProfile("front", 0, DefaultDurationToCaptureAfterEvent)
	if err != nil {
		return nil, fmt.Errorf("error opening front camera (0): %w", err)
	}
	back, err := NewVideoProfile("back", 1, DefaultDurationToCaptureAfterEvent)
	if err != nil {
		return nil, fmt.Errorf("error opening back camera (1): %w", err)
	}
	v := &VideoProfiles{
		front: front,
		back:  back,
	}
	return v, nil
}

func (v *VideoProfiles) Start(frontWindow, backWindow *VideoPlaybackWindow) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		v.front.Start(DefaultSecondsToRecord, frontWindow)
	}()
	go func() {
		defer wg.Done()
		go v.back.Start(DefaultSecondsToRecord, backWindow)
	}()
	wg.Wait()
}

func (v *VideoProfiles) Save(detection Detection) {
	go v.front.Save(detection)
	go v.back.Save(detection)
}

func (v *VideoProfiles) Stop() {
	v.front.Stop()
	v.back.Stop()
}

type VideoProfile struct {
	name                        string
	cam                         *gocv.VideoCapture
	durationToCaptureAfterEvent time.Duration

	stop chan struct{}
	save chan struct{}
}

func NewVideoProfile(name string, device int, durationToCaptureAfterEvent time.Duration) (*VideoProfile, error) {
	cam, err := gocv.VideoCaptureDevice(device)
	if err != nil {
		return nil, fmt.Errorf("error opening front camera (0): %w", err)
	}

	cam.Set(gocv.VideoCaptureFrameWidth, float64(DefaultCamWidth))
	cam.Set(gocv.VideoCaptureFrameHeight, float64(DefaultCamHeight))
	cam.Set(gocv.VideoCaptureFPS, DefaultFPS)

	// Retrieve camera properties
	width := cam.Get(gocv.VideoCaptureFrameWidth)
	height := cam.Get(gocv.VideoCaptureFrameHeight)
	fps := cam.Get(gocv.VideoCaptureFPS)

	// Print camera details
	fmt.Println("==================================================")
	fmt.Printf("Camera Details %s (device=%d):\n", name, device)
	fmt.Printf("Resolution: %.0fx%.0f\n", width, height)
	fmt.Printf("FPS: %.2f\n", fps)
	fmt.Println("==================================================")

	return &VideoProfile{
		name:                        name,
		cam:                         cam,
		durationToCaptureAfterEvent: durationToCaptureAfterEvent,

		stop: make(chan struct{}),
		save: make(chan struct{}),
	}, nil
}

func (v *VideoProfile) Start(secondsToRecord int, window *VideoPlaybackWindow) (err error) {
	fmt.Printf(">>>>>>>> starting video capture for %s\n", v.name)
	frameBuffer := NewVideoFrameBuffer(int(DefaultFPS) * secondsToRecord)

	frame := gocv.NewMat()
	defer frame.Close()

	var playback *VideoPlayback

	var stopped bool
	for !stopped {
		select {
		case <-v.stop:
			stopped = true
		case <-v.save:
			// stop playback if playback is running
			if playback != nil {
				playback.Stop()
			}
			fmt.Printf("saving video for %s\n", v.name)

			// Format time to a readable format
			t := time.Now().Format("2006-01-02 15-04-05")
			file := fmt.Sprintf("videos/%s %s.avi", t, v.name)
			if err := frameBuffer.Save(
				file,
				int(v.cam.Get(gocv.VideoCaptureFrameWidth)),
				int(v.cam.Get(gocv.VideoCaptureFrameHeight)),
				float64(v.cam.Get(gocv.VideoCaptureFPS)),
			); err != nil {
				fmt.Printf("error saving video: %v\n", err)
				continue
			}

			var err error
			playback, err = NewVideoPlayback(v.name, file, DefaultFPS)
			if err != nil {
				fmt.Printf("error creating capture: %v\n", err)
				continue
			}
			go playback.Start(DefaultPlaybackSpeed, window)

		default:
			if ok := v.cam.Read(&frame); !ok {
				continue
			}
			cloned := gocv.NewMat()
			// Rotate the frame by 180 degrees
			gocv.Rotate(frame, &cloned, gocv.Rotate180Clockwise)

			frameBuffer.Append(cloned)
		}
	}
	fmt.Printf(">>>>>>>> video profile capturing stopped for camera %s\n", v.name)
	return nil
}

func (v *VideoProfile) Stop() {
	v.stop <- struct{}{}
}

func (v *VideoProfile) Save(detection Detection) {
	elapsed := time.Since(detection.DetectionTime)
	delay := v.durationToCaptureAfterEvent - elapsed
	fmt.Printf("delaying saving video by %s\n", delay)
	time.Sleep(delay)
	v.save <- struct{}{}
}

type VideoFrameBuffer struct {
	sync.RWMutex

	frames []gocv.Mat
	idx    int
}

// 120 FPS -> to keep 3 seconds before and after impact -> 720 frames
func NewVideoFrameBuffer(maxFrames int) *VideoFrameBuffer {
	return &VideoFrameBuffer{
		frames: make([]gocv.Mat, maxFrames),
	}
}

func (v *VideoFrameBuffer) Append(frame gocv.Mat) {
	v.Lock()
	defer v.Unlock()

	if v.idx < len(v.frames) {
		v.frames[v.idx] = frame
		v.idx++
	} else {
		v.frames[0].Close()
		v.frames = append(v.frames[1:], frame)
	}
}
func (v *VideoFrameBuffer) Save(file string, width, height int, fps float64) (err error) {
	v.RLock()
	defer v.RUnlock()

	if !v.Full() {
		return fmt.Errorf("video frame buffer is not full (%d/%d)", v.idx, len(v.frames))
	}
	fmt.Printf("--------------------------------------------------\n")
	fmt.Printf("video frame buffer is full (%d)\nparameters:\n\twidth: %d\n\theight: %d\n\tfps: %f\n",
		len(v.frames), width, height, fps)
	fmt.Printf("--------------------------------------------------\n")

	videoWriter, err := gocv.VideoWriterFile(file, "MJPG", fps, width, height, true)
	if err != nil {
		return fmt.Errorf("error creating video writer: %w", err)
	}
	defer videoWriter.Close()

	for idx, frame := range v.frames {
		err = videoWriter.Write(frame)
		if err != nil {
			return fmt.Errorf("error writing frame (%d): %w", idx, err)
		}
	}

	return nil
}
func (v *VideoFrameBuffer) Full() bool {
	return v.idx == len(v.frames)
}

type VideoPlayback struct {
	camName string
	file    string
	fps     float64

	stop chan struct{}
}

func NewVideoPlayback(camName string, file string, fps float64) (*VideoPlayback, error) {
	v := &VideoPlayback{
		camName: camName,
		file:    file,
		fps:     fps,
		stop:    make(chan struct{}),
	}
	return v, nil
}

// playbackSpeed is an integer > 0, 0.5 is half speed, 1 is normal speed, 2 is double speed
// window has to be passed in as must run on the main thread.
func (v *VideoPlayback) Start(playbackSpeed float64, window *VideoPlaybackWindow) {
	// Create a Mat to hold the video frames
	f := gocv.NewMat()
	defer f.Close()

	// compute time to delay between frames
	frameDelay := float64(time.Second) / v.fps / playbackSpeed

	for {
		// Open the video file
		video, err := gocv.VideoCaptureFile(v.file)
		if err != nil {
			fmt.Printf("Error opening video file %s: %v\n", v.file, err)
			return
		}
		for {
			// Read a frame from the video
			if ok := video.Read(&f); !ok {
				fmt.Printf(">>>>>>>> Restarting %s video playback\n", v.camName)
				break
			}
			if f.Empty() {
				continue
			}

			select {
			case <-v.stop:
				fmt.Printf(">>>>>>>> %s video playback stopped\n", v.camName)
				return
			default:
			}

			// Display the frame in the window

			window.Input() <- f

			time.Sleep(time.Duration(frameDelay))
		}
		video.Close()
	}
}
func (v *VideoPlayback) Stop() {
	fmt.Printf("stopping %s video playback\n", v.camName)
	v.stop <- struct{}{}
}

type VideoPlaybackWindow struct {
	*gocv.Window
	frames chan gocv.Mat
}

func NewVideoPlaybackWindow(name string) *VideoPlaybackWindow {
	return &VideoPlaybackWindow{
		Window: gocv.NewWindow(name),
		frames: make(chan gocv.Mat),
	}
}
func (v *VideoPlaybackWindow) PlayNextFrame() {
	select {
	case frame := <-v.frames:
		v.Window.IMShow(frame)
		if key := gocv.WaitKey(1); key == 'q' {
			break
		}
	default:
	}
}
func (v *VideoPlaybackWindow) Input() chan<- gocv.Mat {
	return v.frames
}
