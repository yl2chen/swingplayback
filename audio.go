package main

import (
	"fmt"
	"math"
	"os"
	"sync"
	"text/template"
	"time"

	"github.com/gordonklaus/portaudio"
)

const (
	DefaultClubStrikeDecibelThreshold = 75.0
)

var tmpl = template.Must(template.New("").Parse(
	`{{. | len}} host APIs: {{range .}}
	Name:                   {{.Name}}
	{{if .DefaultInputDevice}}Default input device:   {{.DefaultInputDevice.Name}}{{end}}
	{{if .DefaultOutputDevice}}Default output device:  {{.DefaultOutputDevice.Name}}{{end}}
	Devices: {{range .Devices}}
		Name:                      {{.Name}}
		MaxInputChannels:          {{.MaxInputChannels}}
		MaxOutputChannels:         {{.MaxOutputChannels}}
		DefaultLowInputLatency:    {{.DefaultLowInputLatency}}
		DefaultLowOutputLatency:   {{.DefaultLowOutputLatency}}
		DefaultHighInputLatency:   {{.DefaultHighInputLatency}}
		DefaultHighOutputLatency:  {{.DefaultHighOutputLatency}}
		DefaultSampleRate:         {{.DefaultSampleRate}}
	{{end}}
{{end}}`,
))

type Audio struct {
	detection        chan time.Time
	decibleThreshold float64
}

func NewAudio(decibelThreshold float64) (*Audio, error) {
	a := &Audio{
		detection:        make(chan time.Time),
		decibleThreshold: decibelThreshold,
	}
	return a, nil
}

func (a *Audio) StartDetection() (err error) {
	// Initialize PortAudio
	if err = portaudio.Initialize(); err != nil {
		return fmt.Errorf("error initializing PortAudio: %w", err)
	}
	defer portaudio.Terminate()

	hs, _ := portaudio.HostApis()
	_ = tmpl.Execute(os.Stdout, hs)

	// Set up audio parameters
	const sampleRate = 44100
	const seconds = 0.1
	const maxSignalLength = sampleRate * seconds
	const channels = 1
	const detectInterval = time.Millisecond * 100

	// Create a buffer to hold the recorded audio
	buffer := make([]int32, 1024)
	var bite []float64
	var mutex sync.RWMutex

	// Open the audio stream
	stream, err := portaudio.OpenDefaultStream(channels, 0, sampleRate, len(buffer), buffer)
	if err != nil {
		return fmt.Errorf("error opening audio stream: %w", err)
	}
	defer stream.Close()

	go func() {
		for {
			stream.Read()
			data := make([]float64, len(buffer))
			for i, frame := range buffer {
				data[i] = float64(frame)
			}
			// append the buffer to the bite
			mutex.Lock()
			bite = append(bite, data...)
			if len(bite) > maxSignalLength {
				bite = bite[len(bite)-maxSignalLength:]
			}
			mutex.Unlock()
		}
	}()

	device, _ := portaudio.DefaultInputDevice()
	fmt.Printf("Default Input Device: %s, Sample Rates: %v\n", device.Name, device.DefaultSampleRate)

	// Start recording
	fmt.Println("Recording audio...", stream.Info().SampleRate)
	if err := stream.Start(); err != nil {
		return fmt.Errorf("error starting audio stream: %w", err)
	}

	detectTicker := time.NewTicker(detectInterval).C
	for range detectTicker {
		mutex.RLock()

		decibels := calculateDecibels(bite)
		fmt.Printf("Sound level: %f dB\n", decibels)
		if decibels > a.decibleThreshold {
			a.detection <- time.Now()
		}

		mutex.RUnlock()
	}
	return nil
}

func (a *Audio) DetectAboveThreshold() <-chan time.Time {
	return a.detection
}

// // normalize converts audio samples to values between -1 and 1
// func normalize(samples []float64) []float64 {
// 	max := 0.0
// 	for _, sample := range samples {
// 		if math.Abs(sample) > max {
// 			max = math.Abs(sample)
// 		}
// 	}
// 	for i := range samples {
// 		samples[i] /= max
// 	}
// 	return samples
// }

// // normalize computes the magnitude of a signal
// func normalize(signal []float64) float64 {
// 	var sum float64
// 	for _, v := range signal {
// 		sum += v * v
// 	}
// 	return math.Sqrt(sum)
// }

// // crossCorrelation computes the cross-correlation between a signal and a pattern
// func crossCorrelation(signal, pattern []float64) []float64 {
// 	if len(pattern) > len(signal) {
// 		return nil
// 	}

// 	result := make([]float64, len(signal)-len(pattern)+1)

// 	for i := 0; i < len(signal)-len(pattern)+1; i++ {
// 		var sum float64
// 		for j := 0; j < len(pattern); j++ {
// 			sum += signal[i+j] * pattern[j]
// 		}
// 		result[i] = sum
// 	}

// 	return result
// }

// // computeSimilarity computes similarity score between signal and pattern, require len(signal) >= len(pattern).
// func computeSimilarity(signal, pattern []float64) (similarity float64) {
// 	// Normalize to determine similarity
// 	signalNorm := normalize(signal)
// 	patternNorm := normalize(pattern)
// 	if signalNorm == 0 || patternNorm == 0 {
// 		return 0
// 	}

// 	correlation := crossCorrelation(signal, pattern)
// 	if len(correlation) == 0 {
// 		return 0
// 	}
// 	max := correlation[0]
// 	for _, value := range correlation {
// 		if value > max {
// 			max = value
// 		}
// 	}
// 	fmt.Println("Max correlation: ", max, signalNorm, patternNorm)
// 	similarity = max / (signalNorm * patternNorm)

// 	return similarity
// }

// calculateDecibels converts RMS to decibels (dB)
func calculateDecibels(signal []float64) float64 {
	rms := calculateRMS(signal)
	// Reference level (16-bit audio: typically max value is 32768)
	const referenceLevel = 32768.0

	if rms == 0 {
		return -math.Inf(1) // Return -Infinity for silence
	}
	return 20 * math.Log10(rms/referenceLevel)
}

// calculateRMS computes the Root Mean Square of the audio samples
func calculateRMS(samples []float64) float64 {
	var sum float64
	for _, sample := range samples {
		sum += float64(sample * sample)
	}
	mean := sum / float64(len(samples))
	return math.Sqrt(mean)
}
