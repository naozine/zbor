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

	// Read and validate RIFF header (12 bytes)
	riffHeader := make([]byte, 12)
	if _, err := io.ReadFull(f, riffHeader); err != nil {
		return nil, 0, fmt.Errorf("failed to read RIFF header: %w", err)
	}

	if string(riffHeader[0:4]) != "RIFF" || string(riffHeader[8:12]) != "WAVE" {
		return nil, 0, fmt.Errorf("not a valid WAV file")
	}

	// Parse chunks to find fmt and data
	var numChannels, sampleRate, bitsPerSample int
	var dataSize int64
	var foundFmt, foundData bool

	for !foundData {
		// Read chunk header (8 bytes: 4 bytes ID + 4 bytes size)
		chunkHeader := make([]byte, 8)
		if _, err := io.ReadFull(f, chunkHeader); err != nil {
			if err == io.EOF {
				break
			}
			return nil, 0, fmt.Errorf("failed to read chunk header: %w", err)
		}

		chunkID := string(chunkHeader[0:4])
		chunkSize := int64(binary.LittleEndian.Uint32(chunkHeader[4:8]))

		switch chunkID {
		case "fmt ":
			// Read format chunk
			fmtData := make([]byte, chunkSize)
			if _, err := io.ReadFull(f, fmtData); err != nil {
				return nil, 0, fmt.Errorf("failed to read fmt chunk: %w", err)
			}
			if len(fmtData) >= 16 {
				numChannels = int(binary.LittleEndian.Uint16(fmtData[2:4]))
				sampleRate = int(binary.LittleEndian.Uint32(fmtData[4:8]))
				bitsPerSample = int(binary.LittleEndian.Uint16(fmtData[14:16]))
			}
			foundFmt = true

		case "data":
			dataSize = chunkSize
			foundData = true
			// Don't read the data here, we'll stream it below

		default:
			// Skip unknown chunks (LIST, INFO, etc.)
			if _, err := f.Seek(chunkSize, io.SeekCurrent); err != nil {
				return nil, 0, fmt.Errorf("failed to skip chunk %s: %w", chunkID, err)
			}
		}

		// WAV chunks are word-aligned (padded to even byte boundary)
		if chunkSize%2 != 0 && chunkID != "data" {
			f.Seek(1, io.SeekCurrent)
		}
	}

	if !foundFmt {
		return nil, 0, fmt.Errorf("fmt chunk not found")
	}
	if !foundData {
		return nil, 0, fmt.Errorf("data chunk not found")
	}

	if bitsPerSample != 16 {
		return nil, 0, fmt.Errorf("only 16-bit WAV files are supported, got %d-bit", bitsPerSample)
	}

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
