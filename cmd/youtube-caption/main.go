package main

import (
	"flag"
	"fmt"
	"os"

	"zbor/internal/youtube"
)

func main() {
	var (
		url         = flag.String("url", "", "YouTube video URL")
		lang        = flag.String("lang", "ja", "Caption language code (default: ja)")
		format      = flag.String("format", "text", "Output format: text, json, srt, vtt")
		outputFile  = flag.String("o", "", "Output file (default: stdout)")
		showInfo    = flag.Bool("info", false, "Show video info only")
		listLangs   = flag.Bool("list", false, "List available captions")
		download    = flag.Bool("download", false, "Download audio")
		audioFormat = flag.String("audio-format", "best", "Audio format: mp4, webm, best")
		audioList   = flag.Bool("audio-list", false, "List available audio formats")
		verbose     = flag.Bool("v", false, "Verbose output")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s -url https://www.youtube.com/watch?v=xxx\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -url https://www.youtube.com/watch?v=xxx -lang en\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -url https://www.youtube.com/watch?v=xxx -format srt -o output.srt\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -url https://www.youtube.com/watch?v=xxx -list\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -url https://www.youtube.com/watch?v=xxx -download -o audio.m4a\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -url https://www.youtube.com/watch?v=xxx -audio-list\n", os.Args[0])
	}

	flag.Parse()

	// Validate input
	if *url == "" {
		fmt.Fprintf(os.Stderr, "Error: YouTube URL is required\n\n")
		flag.Usage()
		os.Exit(1)
	}

	// Create client
	client := youtube.NewClient()

	if *verbose {
		fmt.Fprintf(os.Stderr, "Fetching video: %s\n", *url)
	}

	// Get video info
	video, err := client.GetVideo(*url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get video: %v\n", err)
		os.Exit(1)
	}

	// Show info only
	if *showInfo {
		printVideoInfo(video)
		return
	}

	// List available audio formats
	if *audioList {
		printVideoInfo(video)
		printAudioFormats(client, *url)
		return
	}

	// Download audio
	if *download {
		downloadAudio(client, video, *url, *audioFormat, *lang, *outputFile, *verbose)
		return
	}

	// List available captions
	if *listLangs {
		printVideoInfo(video)
		printCaptionList(video)
		return
	}

	// Validate format for captions
	validFormats := map[string]bool{"text": true, "json": true, "srt": true, "vtt": true}
	if !validFormats[*format] {
		fmt.Fprintf(os.Stderr, "Error: Invalid format '%s'. Must be: text, json, srt, or vtt\n", *format)
		os.Exit(1)
	}

	// Check if captions are available
	if !video.HasCaptions() {
		fmt.Fprintf(os.Stderr, "Error: No captions available for this video\n")
		os.Exit(1)
	}

	if *verbose {
		fmt.Fprintf(os.Stderr, "Fetching captions (lang: %s)...\n", *lang)
	}

	// Fetch captions
	result, err := client.FetchCaption(video, *lang)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to fetch captions: %v\n", err)
		os.Exit(1)
	}

	if *verbose {
		fmt.Fprintf(os.Stderr, "Fetched %d caption entries\n", len(result.Entries))
	}

	// Format output
	var output string
	switch *format {
	case "json":
		output, err = result.FormatAsJSON()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to format JSON: %v\n", err)
			os.Exit(1)
		}
	case "srt":
		output = result.FormatAsSRT()
	case "vtt":
		output = result.FormatAsVTT()
	default:
		output = result.FormatAsText()
	}

	// Write output
	if *outputFile != "" {
		if err := os.WriteFile(*outputFile, []byte(output), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to write output file: %v\n", err)
			os.Exit(1)
		}
		if *verbose {
			fmt.Fprintf(os.Stderr, "Output written to: %s\n", *outputFile)
		}
	} else {
		fmt.Println(output)
	}
}

func printVideoInfo(video *youtube.VideoInfo) {
	fmt.Println("=== Video Info ===")
	fmt.Printf("Title:    %s\n", video.Title)
	fmt.Printf("Author:   %s\n", video.Author)
	fmt.Printf("Duration: %s\n", video.Duration)
	fmt.Printf("ID:       %s\n", video.ID)
}

func printCaptionList(video *youtube.VideoInfo) {
	fmt.Println("\n=== Available Captions ===")
	if len(video.Captions) == 0 {
		fmt.Println("No captions available")
		return
	}
	for i, caption := range video.Captions {
		fmt.Printf("%d. %s (%s)\n", i+1, caption.LanguageCode, caption.Name)
	}
}

func printAudioFormats(client *youtube.Client, url string) {
	formats, err := client.GetAudioFormats(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get audio formats: %v\n", err)
		return
	}

	fmt.Println("\n=== Available Audio Formats ===")
	if len(formats) == 0 {
		fmt.Println("No audio formats available")
		return
	}
	for i, f := range formats {
		size := float64(f.ContentLength) / 1024 / 1024
		langInfo := ""
		if f.Language != "" || f.LanguageName != "" {
			langInfo = fmt.Sprintf(" [%s] (id:%s)", f.LanguageName, f.Language)
			if f.IsDefault {
				langInfo += " (default)"
			}
		}
		fmt.Printf("%d. itag=%d %s %dkbps %.1fMB%s\n",
			i+1, f.ItagNo, f.MimeType, f.Bitrate/1000, size, langInfo)
	}
}

func downloadAudio(client *youtube.Client, video *youtube.VideoInfo, url, audioFormat, lang, outputFile string, verbose bool) {
	// Validate audio format
	validFormats := map[string]bool{"mp4": true, "webm": true, "best": true}
	if !validFormats[audioFormat] {
		fmt.Fprintf(os.Stderr, "Error: Invalid audio format '%s'. Must be: mp4, webm, or best\n", audioFormat)
		os.Exit(1)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Downloading audio (format: %s, lang: %s)...\n", audioFormat, lang)
	}

	opts := &youtube.DownloadAudioOptions{
		Format:     audioFormat,
		Language:   lang,
		OutputPath: outputFile,
	}

	// Progress callback
	var lastPercent int
	progress := func(current, total int64) {
		if total > 0 {
			percent := int(current * 100 / total)
			if percent != lastPercent && percent%10 == 0 {
				fmt.Fprintf(os.Stderr, "  %d%%\n", percent)
				lastPercent = percent
			}
		}
	}

	var err error
	if verbose {
		err = client.DownloadAudioWithProgress(url, opts, progress)
	} else {
		err = client.DownloadAudio(url, opts)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to download audio: %v\n", err)
		os.Exit(1)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Download completed\n")
	}
}
