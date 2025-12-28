package main

import (
	"flag"
	"fmt"
	"os"

	"zbor/internal/youtube"
)

func main() {
	var (
		url        = flag.String("url", "", "YouTube video URL")
		lang       = flag.String("lang", "ja", "Caption language code (default: ja)")
		format     = flag.String("format", "text", "Output format: text, json, srt, vtt")
		outputFile = flag.String("o", "", "Output file (default: stdout)")
		showInfo   = flag.Bool("info", false, "Show video info only")
		listLangs  = flag.Bool("list", false, "List available captions")
		verbose    = flag.Bool("v", false, "Verbose output")
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
	}

	flag.Parse()

	// Validate input
	if *url == "" {
		fmt.Fprintf(os.Stderr, "Error: YouTube URL is required\n\n")
		flag.Usage()
		os.Exit(1)
	}

	// Validate format
	validFormats := map[string]bool{"text": true, "json": true, "srt": true, "vtt": true}
	if !validFormats[*format] {
		fmt.Fprintf(os.Stderr, "Error: Invalid format '%s'. Must be: text, json, srt, or vtt\n", *format)
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

	// List available captions
	if *listLangs {
		printVideoInfo(video)
		printCaptionList(video)
		return
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
