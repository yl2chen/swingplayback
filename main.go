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
	go audio.StartDetection()

	// start video recording
	video, err := NewVideoProfiles()
	if err != nil {
		fmt.Printf("Error creating video profiles: %v\n", err)
		return
	}

	// detect club strikes using high decibel as proxy
	go func() {
		for range audio.DetectAboveThreshold() {
			fmt.Println("High decibel sound bite detected, saving videos...")
			video.Save()
		}
	}()

	// go func() {
	// 	for {
	// 		time.Sleep(10 * time.Second)
	// 		video.Save()
	// 	}
	// }()

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
