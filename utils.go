package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
)

func getVideoAspectRatio(filePath string) (string, error) {

	ffprobe := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)

	var output bytes.Buffer
	ffprobe.Stdout = &output

	err := ffprobe.Run()
	if err != nil {
		fmt.Println("Error running ffprobe:", err)
		return "", err
	}

	// Define a struct to parse only the relevant fields
	type Stream struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	}
	type FFProbeOutput struct {
		Streams []Stream `json:"streams"`
	}

	// Parse JSON output
	var result FFProbeOutput
	err = json.Unmarshal(output.Bytes(), &result)
	if err != nil {
		fmt.Println("Error parsing JSON:", err)
		return "", err
	}

	if len(result.Streams) == 0 {
		fmt.Println("Invalid ffprobe output", err)
		return "", err
	}

	width := result.Streams[0].Width
	height := result.Streams[0].Height

	aspectRatio := determineAspectRatio(width, height)

	return aspectRatio, nil
}

func determineAspectRatio(width, height int) string {
	ratio := float64(width) / float64(height)

	const tolerance = 0.05 // allow small floating-point deviations

	if math.Abs(ratio-(16.0/9.0)) < tolerance {
		return "16:9"
	} else if math.Abs(ratio-(9.0/16.0)) < tolerance {
		return "9:16"
	}
	return "other"
}

func processVideoForFastStart(filePath string) (string, error) {

	outputPath := filePath + ".processing"

	ffmpeg := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outputPath)

	err := ffmpeg.Run()
	if err != nil {
		return "", err
	}

	return outputPath, nil
}
