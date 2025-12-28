package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"zbor/internal/webfetch"
)

func main() {
	var (
		url         = flag.String("url", "", "Target URL")
		format      = flag.String("format", "markdown", "Output format: markdown, html, json")
		outputFile  = flag.String("o", "", "Output file (default: stdout)")
		stealth     = flag.Bool("stealth", true, "Enable stealth mode (bot detection evasion)")
		blockAds    = flag.Bool("block-ads", false, "Block ads")
		blockImages = flag.Bool("block-images", false, "Block images")
		waitTime    = flag.Int("wait", 0, "Wait time in milliseconds")
		selector    = flag.String("selector", "", "Wait for selector")
		timeout     = flag.Int("timeout", 60, "Timeout in seconds")
		verbose     = flag.Bool("v", false, "Verbose output")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s -url https://example.com\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -url https://example.com -format html\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -url https://example.com -o output.md\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -url https://example.com -stealth -block-ads\n", os.Args[0])
	}

	flag.Parse()

	// Validate input
	if *url == "" {
		fmt.Fprintf(os.Stderr, "Error: URL is required\n\n")
		flag.Usage()
		os.Exit(1)
	}

	// Validate format
	validFormats := map[string]bool{"markdown": true, "html": true, "json": true}
	if !validFormats[*format] {
		fmt.Fprintf(os.Stderr, "Error: Invalid format '%s'. Must be: markdown, html, or json\n", *format)
		os.Exit(1)
	}

	if *verbose {
		fmt.Fprintf(os.Stderr, "Fetching: %s\n", *url)
	}

	// Create client
	clientOpts := &webfetch.Options{
		Stealth: *stealth,
	}
	client, err := webfetch.NewClient(clientOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to create client: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	// Build fetch options
	fetchOpts := &webfetch.FetchOptions{
		BlockAds:    *blockAds,
		BlockImages: *blockImages,
		Selector:    *selector,
	}
	if *waitTime > 0 {
		fetchOpts.WaitTime = time.Duration(*waitTime) * time.Millisecond
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Second)
	defer cancel()

	// Fetch
	var result *webfetch.Result
	if *format == "html" {
		result, err = client.FetchHTML(ctx, *url, fetchOpts)
	} else {
		result, err = client.FetchMarkdown(ctx, *url, fetchOpts)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to fetch: %v\n", err)
		os.Exit(1)
	}

	if *verbose {
		fmt.Fprintf(os.Stderr, "Fetched in %.2f seconds\n", result.Duration.Seconds())
		fmt.Fprintf(os.Stderr, "Final URL: %s\n", result.URL)
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
