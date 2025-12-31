package asr

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
)

// ComputeWaveformPeaks reads a WAV file and computes peak amplitudes
// Returns peaks (normalized 0-1), duration in seconds, and error
func ComputeWaveformPeaks(wavPath string, samplesPerSec float64) ([]float64, float64, error) {
	f, err := os.Open(wavPath)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	// Read WAV header
	header := make([]byte, 44)
	if _, err := io.ReadFull(f, header); err != nil {
		return nil, 0, fmt.Errorf("failed to read WAV header: %w", err)
	}

	// Validate RIFF header
	if string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		return nil, 0, fmt.Errorf("not a valid WAV file")
	}

	// Parse format chunk
	// Note: This assumes standard WAV format. For more complex files, we'd need to search for chunks.
	numChannels := int(binary.LittleEndian.Uint16(header[22:24]))
	sampleRate := int(binary.LittleEndian.Uint32(header[24:28]))
	bitsPerSample := int(binary.LittleEndian.Uint16(header[34:36]))

	if bitsPerSample != 16 {
		return nil, 0, fmt.Errorf("only 16-bit WAV files are supported, got %d-bit", bitsPerSample)
	}

	// Get file size to calculate duration
	fileInfo, err := f.Stat()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get file info: %w", err)
	}

	dataSize := fileInfo.Size() - 44 // Subtract header size
	bytesPerSample := bitsPerSample / 8
	totalSamples := int(dataSize) / (bytesPerSample * numChannels)
	duration := float64(totalSamples) / float64(sampleRate)

	// Calculate number of peaks
	numPeaks := int(duration * samplesPerSec)
	if numPeaks <= 0 {
		numPeaks = 1
	}

	samplesPerPeak := totalSamples / numPeaks
	if samplesPerPeak <= 0 {
		samplesPerPeak = 1
	}

	peaks := make([]float64, numPeaks)

	// Read audio data and compute peaks
	buffer := make([]byte, samplesPerPeak*bytesPerSample*numChannels)
	maxAmplitude := float64(1 << 15) // Max value for 16-bit signed integer

	for i := 0; i < numPeaks; i++ {
		n, err := f.Read(buffer)
		if err != nil && err != io.EOF {
			return nil, 0, fmt.Errorf("failed to read audio data: %w", err)
		}
		if n == 0 {
			break
		}

		// Find peak in this chunk
		var maxVal float64
		numSamplesRead := n / (bytesPerSample * numChannels)

		for j := 0; j < numSamplesRead; j++ {
			// Read first channel only for simplicity
			offset := j * bytesPerSample * numChannels
			if offset+1 >= n {
				break
			}
			sample := int16(binary.LittleEndian.Uint16(buffer[offset : offset+2]))
			absVal := math.Abs(float64(sample))
			if absVal > maxVal {
				maxVal = absVal
			}
		}

		peaks[i] = maxVal / maxAmplitude
	}

	return peaks, duration, nil
}
