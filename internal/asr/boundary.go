package asr

// BoundaryAdjustmentParams contains parameters for boundary adjustment
type BoundaryAdjustmentParams struct {
	Threshold    float64 // Audio detection threshold (0-1), default 0.03
	MergeGapMs   int     // Merge clusters within this gap (ms), default 300
	SearchWindow int     // Search window before/after segment (ms), default 1000
}

// DefaultBoundaryParams returns default boundary adjustment parameters
func DefaultBoundaryParams() BoundaryAdjustmentParams {
	return BoundaryAdjustmentParams{
		Threshold:    0.03,
		MergeGapMs:   300,
		SearchWindow: 1000,
	}
}

// AudioCluster represents a contiguous region of audio activity
type AudioCluster struct {
	StartTime float64 // Start time in seconds
	EndTime   float64 // End time in seconds
	MaxPeak   float64 // Maximum peak value in this cluster
}

// BoundaryAdjustmentResult contains the result of boundary adjustment
type BoundaryAdjustmentResult struct {
	OriginalStart   float64        // Original segment start time
	OriginalEnd     float64        // Original segment end time
	AdjustedStart   float64        // Adjusted start time
	AdjustedEnd     float64        // Adjusted end time
	StartExtendedMs int            // How much start was extended (negative = earlier)
	EndExtendedMs   int            // How much end was extended (positive = later)
	MergedClusters  []AudioCluster // Clusters that were merged
}

// FindAudioClusters detects audio clusters in waveform data
// peaks: waveform peak values (0-1)
// samplesPerSec: number of peaks per second
// startTime, endTime: time range to search (seconds)
// threshold: minimum peak value to consider as audio
func FindAudioClusters(peaks []float64, samplesPerSec float64, startTime, endTime float64, threshold float64) []AudioCluster {
	if len(peaks) == 0 || samplesPerSec <= 0 {
		return nil
	}

	startIdx := int(startTime * samplesPerSec)
	endIdx := int(endTime * samplesPerSec)

	// Clamp to valid range
	if startIdx < 0 {
		startIdx = 0
	}
	if endIdx > len(peaks) {
		endIdx = len(peaks)
	}
	if startIdx >= endIdx {
		return nil
	}

	var clusters []AudioCluster
	var currentCluster *AudioCluster

	for i := startIdx; i < endIdx; i++ {
		peak := peaks[i]
		time := float64(i) / samplesPerSec

		if peak >= threshold {
			// Audio detected
			if currentCluster == nil {
				// Start new cluster
				currentCluster = &AudioCluster{
					StartTime: time,
					EndTime:   time,
					MaxPeak:   peak,
				}
			} else {
				// Extend current cluster
				currentCluster.EndTime = time
				if peak > currentCluster.MaxPeak {
					currentCluster.MaxPeak = peak
				}
			}
		} else {
			// Silence
			if currentCluster != nil {
				// End current cluster
				clusters = append(clusters, *currentCluster)
				currentCluster = nil
			}
		}
	}

	// Don't forget the last cluster
	if currentCluster != nil {
		clusters = append(clusters, *currentCluster)
	}

	return clusters
}

// MergeClusters merges clusters that are within mergeGapMs of each other
func MergeClusters(clusters []AudioCluster, mergeGapMs int) []AudioCluster {
	if len(clusters) == 0 {
		return nil
	}

	mergeGapSec := float64(mergeGapMs) / 1000.0
	var merged []AudioCluster
	current := clusters[0]

	for i := 1; i < len(clusters); i++ {
		next := clusters[i]
		gap := next.StartTime - current.EndTime

		if gap <= mergeGapSec {
			// Merge with current
			current.EndTime = next.EndTime
			if next.MaxPeak > current.MaxPeak {
				current.MaxPeak = next.MaxPeak
			}
		} else {
			// Gap too large, save current and start new
			merged = append(merged, current)
			current = next
		}
	}
	merged = append(merged, current)

	return merged
}

// AdjustBoundaries adjusts segment boundaries based on waveform data
// peaks: waveform peak values (0-1)
// samplesPerSec: number of peaks per second (e.g., 50)
// segmentStart, segmentEnd: original segment time range (seconds)
// params: adjustment parameters
func AdjustBoundaries(peaks []float64, samplesPerSec float64, segmentStart, segmentEnd float64, params BoundaryAdjustmentParams) BoundaryAdjustmentResult {
	result := BoundaryAdjustmentResult{
		OriginalStart: segmentStart,
		OriginalEnd:   segmentEnd,
		AdjustedStart: segmentStart,
		AdjustedEnd:   segmentEnd,
	}

	if len(peaks) == 0 {
		return result
	}

	searchWindowSec := float64(params.SearchWindow) / 1000.0
	mergeGapSec := float64(params.MergeGapMs) / 1000.0

	// Search before segment
	searchStartBefore := segmentStart - searchWindowSec
	if searchStartBefore < 0 {
		searchStartBefore = 0
	}
	clustersBefore := FindAudioClusters(peaks, samplesPerSec, searchStartBefore, segmentStart, params.Threshold)
	clustersBefore = MergeClusters(clustersBefore, params.MergeGapMs)

	// Search after segment
	totalDuration := float64(len(peaks)) / samplesPerSec
	searchEndAfter := segmentEnd + searchWindowSec
	if searchEndAfter > totalDuration {
		searchEndAfter = totalDuration
	}
	clustersAfter := FindAudioClusters(peaks, samplesPerSec, segmentEnd, searchEndAfter, params.Threshold)
	clustersAfter = MergeClusters(clustersAfter, params.MergeGapMs)

	// Find clusters within segment (for reference)
	clustersWithin := FindAudioClusters(peaks, samplesPerSec, segmentStart, segmentEnd, params.Threshold)
	clustersWithin = MergeClusters(clustersWithin, params.MergeGapMs)

	// Adjust start: find last cluster before segment that's within merge gap
	newStart := segmentStart
	var mergedBefore []AudioCluster
	for i := len(clustersBefore) - 1; i >= 0; i-- {
		cluster := clustersBefore[i]
		gap := newStart - cluster.EndTime
		if gap <= mergeGapSec {
			newStart = cluster.StartTime
			mergedBefore = append([]AudioCluster{cluster}, mergedBefore...)
		} else {
			break // Gap too large, stop searching
		}
	}

	// Adjust end: find first cluster after segment that's within merge gap
	newEnd := segmentEnd
	var mergedAfter []AudioCluster
	for _, cluster := range clustersAfter {
		gap := cluster.StartTime - newEnd
		if gap <= mergeGapSec {
			newEnd = cluster.EndTime
			mergedAfter = append(mergedAfter, cluster)
		} else {
			break // Gap too large, stop searching
		}
	}

	result.AdjustedStart = newStart
	result.AdjustedEnd = newEnd
	result.StartExtendedMs = int((segmentStart - newStart) * 1000)
	result.EndExtendedMs = int((newEnd - segmentEnd) * 1000)

	// Collect all merged clusters
	result.MergedClusters = append(result.MergedClusters, mergedBefore...)
	result.MergedClusters = append(result.MergedClusters, clustersWithin...)
	result.MergedClusters = append(result.MergedClusters, mergedAfter...)

	return result
}
