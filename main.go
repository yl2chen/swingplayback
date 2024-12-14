package main

import (
	"fmt"
)

func main() {

	for {
		start()
	}
}

func start() {
	// start audio streaming
	audio, err := NewAudio(DefaultClubStrikeDecibelThreshold)
	if err != nil {
		fmt.Printf("Error creating audio: %v\n", err)
		return
	}
	go audio.StartDetection(DefaultMinDetectionInterval)

	// start video recording
	video, err := NewVideoProfiles()
	if err != nil {
		fmt.Printf("Error creating video profiles: %v\n", err)
		return
	}

	// detect club strikes using high decibel as proxy
	go func() {
		for detection := range audio.DetectAboveThreshold() {
			fmt.Printf(">>>>>>>> High decibel sound bite detected (%f DB @ %s), saving videos...\n",
				detection.Decibel, detection.DetectionTime.Format("15:04:05"))
			go video.Save(detection)
		}
	}()

	// Create a window to display the video
	windowFront := NewVideoPlaybackWindow("Video Player Front")
	defer windowFront.Close()
	// Create a window to display the video
	windowBack := NewVideoPlaybackWindow("Video Player Back")
	defer windowBack.Close()

	go video.Start(windowFront, windowBack)

	for {
		windowFront.PlayNextFrame()
		windowBack.PlayNextFrame()
	}
}
