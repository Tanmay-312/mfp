package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Song represents a single song
type Song struct {
	Title    string `json:"title"`
	VideoID  string `json:"video_id"`
	Duration string `json:"duration"`
	URL      string `json:"url"`
}

// Playlist represents a YouTube playlist
type Playlist struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Songs       []Song `json:"songs"`
	LastUpdated string `json:"last_updated"`
}

// PlayerState holds the current state of the music player
type PlayerState struct {
	CurrentPlaylist  string    `json:"current_playlist"`
	CurrentSongIndex int       `json:"current_song_index"`
	IsPlaying        bool      `json:"is_playing"`
	IsShuffle        bool      `json:"is_shuffle"`
	IsLoop           bool      `json:"is_loop"`
	Volume           int       `json:"volume"`
	ShuffleOrder     []int     `json:"shuffle_order"`
	ShuffleIndex     int       `json:"shuffle_index"`
	LastUpdated      time.Time `json:"last_updated"`
	Position         int       `json:"position"` // Current position in seconds
}

// Config holds application configuration
type Config struct {
	DataDir    string
	StateFile  string
	SocketFile string
	Playlists  map[string]*Playlist
	State      *PlayerState
}

var (
	config      *Config
	currentCmd  *exec.Cmd
	quitChannel = make(chan bool)
	skipChannel = make(chan bool)
)

func main() {
	// Initialize configuration
	var err error
	config, err = initConfig()
	if err != nil {
		log.Fatal("Failed to initialize config:", err)
	}

	// Handle command line arguments
	if len(os.Args) < 2 {
		showHelp()
		return
	}

	command := os.Args[1]
	args := os.Args[2:]

	// Set up signal handling for graceful shutdown
	setupSignalHandler()

	switch command {
	case "add":
		handleAdd(args)
	case "play":
		handlePlay(args)
	case "stop":
		handleStop()
	case "next":
		handleNext()
	case "prev", "previous":
		handlePrevious()
	case "current", "now":
		handleCurrent()
	case "queue":
		handleQueue(args)
	case "jump":
		handleJump(args)
	case "shuffle":
		handleShuffle(args)
	case "loop":
		handleLoop(args)
	case "volume", "vol":
		handleVolume(args)
	case "seek":
		handleSeek(args)
	case "list", "playlists":
		handleListPlaylists()
	case "songs":
		handleListSongs(args)
	case "rename":
		handleRename(args)
	case "delete", "remove":
		handleDelete(args)
	case "help", "-h", "--help":
		showHelp()
	case "status":
		handleStatus()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		showHelp()
	}
}

func initConfig() (*Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	dataDir := filepath.Join(homeDir, ".mfp")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	stateFile := filepath.Join(dataDir, "state.json")
	socketFile := filepath.Join(dataDir, "mpv-socket")
	playlistsFile := filepath.Join(dataDir, "playlists.json")

	config := &Config{
		DataDir:    dataDir,
		StateFile:  stateFile,
		SocketFile: socketFile,
		Playlists:  make(map[string]*Playlist),
		State: &PlayerState{
			Volume:           70,
			CurrentSongIndex: 0,
			ShuffleOrder:     []int{},
			ShuffleIndex:     0,
			Position:         0,
		},
	}

	// Load existing playlists
	if data, err := ioutil.ReadFile(playlistsFile); err == nil {
		json.Unmarshal(data, &config.Playlists)
	}

	// Load existing state
	if data, err := ioutil.ReadFile(stateFile); err == nil {
		json.Unmarshal(data, config.State)
	}

	return config, nil
}

func saveConfig() error {
	playlistsFile := filepath.Join(config.DataDir, "playlists.json")
	data, err := json.MarshalIndent(config.Playlists, "", "  ")
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(playlistsFile, data, 0644); err != nil {
		return err
	}

	config.State.LastUpdated = time.Now()
	stateData, err := json.MarshalIndent(config.State, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(config.StateFile, stateData, 0644)
}

func setupSignalHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		cleanup()
		os.Exit(0)
	}()
}

func cleanup() {
	if currentCmd != nil && currentCmd.Process != nil {
		// Send quit command to mpv
		sendMpvCommand("quit")
		currentCmd.Process.Kill()
	}
	config.State.IsPlaying = false
	saveConfig()
	// Clean up socket file
	os.Remove(config.SocketFile)
}

func handleAdd(args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: mfp add <playlist_name> <youtube_playlist_url>")
		return
	}

	name := args[0]
	url := args[1]

	// Validate YouTube playlist URL
	if !isValidPlaylistURL(url) {
		fmt.Println("Error: Invalid YouTube playlist URL")
		return
	}

	fmt.Printf("Adding playlist '%s'...\n", name)

	// Extract playlist ID from URL
	playlistID := extractPlaylistID(url)
	if playlistID == "" {
		fmt.Println("Error: Could not extract playlist ID from URL")
		return
	}

	// Fetch playlist information using yt-dlp
	songs, err := fetchPlaylistSongs(playlistID)
	if err != nil {
		fmt.Printf("Error fetching playlist: %v\n", err)
		return
	}

	playlist := &Playlist{
		Name:        name,
		URL:         url,
		Songs:       songs,
		LastUpdated: time.Now().Format("2006-01-02 15:04:05"),
	}

	config.Playlists[name] = playlist
	if err := saveConfig(); err != nil {
		fmt.Printf("Error saving playlist: %v\n", err)
		return
	}

	fmt.Printf("Successfully added playlist '%s' with %d songs\n", name, len(songs))
}

func handleStop() {
	if currentCmd != nil && currentCmd.Process != nil {
		// Send quit command to mpv first for graceful shutdown
		sendMpvCommand("quit")

		// Wait a moment for graceful shutdown
		time.Sleep(100 * time.Millisecond)

		// Force kill if still running
		if currentCmd.Process != nil {
			currentCmd.Process.Kill()
		}
		currentCmd = nil
	}

	config.State.IsPlaying = false
	config.State.Position = 0
	saveConfig()

	// Clean up socket file
	os.Remove(config.SocketFile)

	fmt.Println("Playback stopped")
}

func handleNext() {
	if !config.State.IsPlaying {
		fmt.Println("No music is currently playing")
		return
	}

	// Update our internal state first
	playlist := config.Playlists[config.State.CurrentPlaylist]
	if playlist != nil {
		if config.State.IsShuffle {
			config.State.ShuffleIndex++
			if config.State.ShuffleIndex >= len(config.State.ShuffleOrder) {
				if config.State.IsLoop {
					config.State.ShuffleIndex = 0
				} else {
					handleStop()
					return
				}
			}
		} else {
			config.State.CurrentSongIndex++
			if config.State.CurrentSongIndex >= len(playlist.Songs) {
				if config.State.IsLoop {
					config.State.CurrentSongIndex = 0
				} else {
					handleStop()
					return
				}
			}
		}
	}

	// Force skip to next song immediately
	sendMpvCommand("playlist-next")
	saveConfig()
	fmt.Println("Skipping to next song...")
}

func handlePrevious() {
	if !config.State.IsPlaying {
		fmt.Println("No music is currently playing")
		return
	}

	// Update our internal state first
	playlist := config.Playlists[config.State.CurrentPlaylist]
	if playlist != nil {
		if config.State.IsShuffle {
			config.State.ShuffleIndex--
			if config.State.ShuffleIndex < 0 {
				if config.State.IsLoop {
					config.State.ShuffleIndex = len(config.State.ShuffleOrder) - 1
				} else {
					config.State.ShuffleIndex = 0
				}
			}
		} else {
			config.State.CurrentSongIndex--
			if config.State.CurrentSongIndex < 0 {
				if config.State.IsLoop {
					config.State.CurrentSongIndex = len(playlist.Songs) - 1
				} else {
					config.State.CurrentSongIndex = 0
				}
			}
		}
	}

	// Force skip to previous song immediately
	sendMpvCommand("playlist-prev")
	saveConfig()
	fmt.Println("Going to previous song...")
}

func handleQueue(args []string) {
	if config.State.CurrentPlaylist == "" {
		fmt.Println("No playlist is currently loaded")
		return
	}

	playlist := config.Playlists[config.State.CurrentPlaylist]
	if playlist == nil {
		fmt.Println("Current playlist not found")
		return
	}

	showCount := 5
	if len(args) > 0 {
		if count, err := strconv.Atoi(args[0]); err == nil && count > 0 {
			showCount = count
		}
	}

	currentIndex := getCurrentSongIndex()
	fmt.Printf("Queue for playlist '%s':\n\n", config.State.CurrentPlaylist)

	// Show previous songs
	fmt.Println("Previous:")
	start := currentIndex - showCount
	if start < 0 {
		start = 0
	}
	for i := start; i < currentIndex; i++ {
		realIndex := i
		if config.State.IsShuffle && i < len(config.State.ShuffleOrder) {
			realIndex = config.State.ShuffleOrder[i]
		}
		if realIndex < len(playlist.Songs) {
			fmt.Printf("  %d. %s\n", i+1, playlist.Songs[realIndex].Title)
		}
	}

	// Show current song
	if currentIndex < len(playlist.Songs) {
		realIndex := currentIndex
		if config.State.IsShuffle && currentIndex < len(config.State.ShuffleOrder) {
			realIndex = config.State.ShuffleOrder[currentIndex]
		}
		if realIndex < len(playlist.Songs) {
			status := "▶"
			if !config.State.IsPlaying {
				status = "⏸"
			}
			fmt.Printf("\n%s %d. %s (NOW PLAYING)\n\n", status, currentIndex+1, playlist.Songs[realIndex].Title)
		}
	}

	// Show next songs
	fmt.Println("Next:")
	end := currentIndex + showCount + 1
	if end > len(playlist.Songs) {
		end = len(playlist.Songs)
	}
	for i := currentIndex + 1; i < end; i++ {
		realIndex := i
		if config.State.IsShuffle && i < len(config.State.ShuffleOrder) {
			realIndex = config.State.ShuffleOrder[i]
		}
		if realIndex < len(playlist.Songs) {
			fmt.Printf("  %d. %s\n", i+1, playlist.Songs[realIndex].Title)
		}
	}
}

func handleJump(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: mfp jump <song_number>")
		return
	}

	if config.State.CurrentPlaylist == "" {
		fmt.Println("No playlist is currently loaded")
		return
	}

	playlist := config.Playlists[config.State.CurrentPlaylist]
	if playlist == nil {
		fmt.Println("Current playlist not found")
		return
	}

	songNum, err := strconv.Atoi(args[0])
	if err != nil || songNum < 1 || songNum > len(playlist.Songs) {
		fmt.Printf("Invalid song number. Please use 1-%d\n", len(playlist.Songs))
		return
	}

	// Convert to 0-based index
	targetIndex := songNum - 1

	if config.State.IsShuffle {
		// Find the shuffle index that corresponds to this song
		for i, shuffledIndex := range config.State.ShuffleOrder {
			if shuffledIndex == targetIndex {
				config.State.ShuffleIndex = i
				break
			}
		}
	} else {
		config.State.CurrentSongIndex = targetIndex
	}

	if config.State.IsPlaying {
		// Jump to the song in mpv playlist
		sendMpvCommand(fmt.Sprintf("set playlist-pos %d", targetIndex))
	}

	fmt.Printf("Jumped to song %d: %s\n", songNum, playlist.Songs[targetIndex].Title)
	saveConfig()
}

func handleShuffle(args []string) {
	if len(args) == 0 {
		// Toggle shuffle
		config.State.IsShuffle = !config.State.IsShuffle
	} else {
		switch strings.ToLower(args[0]) {
		case "on", "true", "1":
			config.State.IsShuffle = true
		case "off", "false", "0":
			config.State.IsShuffle = false
		default:
			fmt.Println("Usage: mfp shuffle [on|off]")
			return
		}
	}

	if config.State.IsShuffle {
		initShuffleOrder()
		if config.State.IsPlaying {
			sendMpvCommand("set shuffle yes")
		}
		fmt.Println("Shuffle: ON")
	} else {
		if config.State.IsPlaying {
			sendMpvCommand("set shuffle no")
		}
		fmt.Println("Shuffle: OFF")
	}

	saveConfig()
}

func handleLoop(args []string) {
	if len(args) == 0 {
		// Toggle loop
		config.State.IsLoop = !config.State.IsLoop
	} else {
		switch strings.ToLower(args[0]) {
		case "on", "true", "1":
			config.State.IsLoop = true
		case "off", "false", "0":
			config.State.IsLoop = false
		default:
			fmt.Println("Usage: mfp loop [on|off]")
			return
		}
	}

	if config.State.IsLoop {
		if config.State.IsPlaying {
			sendMpvCommand("set loop-playlist inf")
		}
		fmt.Println("Loop: ON")
	} else {
		if config.State.IsPlaying {
			sendMpvCommand("set loop-playlist no")
		}
		fmt.Println("Loop: OFF")
	}

	saveConfig()
}

func handleVolume(args []string) {
	if len(args) == 0 {
		fmt.Printf("Current volume: %d%%\n", config.State.Volume)
		return
	}

	switch args[0] {
	case "up", "+":
		config.State.Volume += 10
		if config.State.Volume > 100 {
			config.State.Volume = 100
		}
	case "down", "-":
		config.State.Volume -= 10
		if config.State.Volume < 0 {
			config.State.Volume = 0
		}
	default:
		if vol, err := strconv.Atoi(args[0]); err == nil {
			if vol >= 0 && vol <= 100 {
				config.State.Volume = vol
			} else {
				fmt.Println("Volume must be between 0 and 100")
				return
			}
		} else {
			fmt.Println("Usage: mfp volume [up|down|<0-100>]")
			return
		}
	}

	// Set volume in mpv if playing
	if config.State.IsPlaying {
		sendMpvCommand(fmt.Sprintf("set volume %d", config.State.Volume))
	}

	fmt.Printf("Volume set to: %d%%\n", config.State.Volume)
	saveConfig()
}

func handleSeek(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: mfp seek [+|-]<seconds>")
		return
	}

	if !config.State.IsPlaying {
		fmt.Println("No music is currently playing")
		return
	}

	seekArg := args[0]
	var seekSeconds int
	var err error
	var relative bool

	if strings.HasPrefix(seekArg, "+") || strings.HasPrefix(seekArg, "-") {
		relative = true
		seekSeconds, err = strconv.Atoi(seekArg[1:])
		if strings.HasPrefix(seekArg, "-") {
			seekSeconds = -seekSeconds
		}
	} else {
		seekSeconds, err = strconv.Atoi(seekArg)
	}

	if err != nil {
		fmt.Println("Invalid seek value")
		return
	}

	if relative {
		sendMpvCommand(fmt.Sprintf("seek %d", seekSeconds))
		if seekSeconds > 0 {
			fmt.Printf("Seeking forward %d seconds\n", seekSeconds)
		} else {
			fmt.Printf("Seeking backward %d seconds\n", -seekSeconds)
		}
	} else {
		sendMpvCommand(fmt.Sprintf("seek %d absolute", seekSeconds))
		fmt.Printf("Seeking to %d seconds\n", seekSeconds)
	}
}

func handleListPlaylists() {
	if len(config.Playlists) == 0 {
		fmt.Println("No playlists found. Add one with: mfp add <name> <url>")
		return
	}

	fmt.Println("Available playlists:")
	for name, playlist := range config.Playlists {
		status := ""
		if name == config.State.CurrentPlaylist {
			if config.State.IsPlaying {
				status = " (currently playing)"
			} else {
				status = " (loaded)"
			}
		}
		fmt.Printf("  %s - %d songs%s\n", name, len(playlist.Songs), status)
		fmt.Printf("    Last updated: %s\n", playlist.LastUpdated)
	}
}

func handleListSongs(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: mfp songs <playlist_name>")
		return
	}

	playlistName := args[0]
	playlist, exists := config.Playlists[playlistName]
	if !exists {
		fmt.Printf("Playlist '%s' not found\n", playlistName)
		return
	}

	fmt.Printf("Songs in playlist '%s':\n", playlistName)
	for i, song := range playlist.Songs {
		fmt.Printf("  %d. %s (%s)\n", i+1, song.Title, song.Duration)
	}
}

func handleRename(args []string) {
	if len(args) != 2 {
		fmt.Println("Usage: mfp rename <old_name> <new_name>")
		return
	}

	oldName := args[0]
	newName := args[1]

	playlist, exists := config.Playlists[oldName]
	if !exists {
		fmt.Printf("Playlist '%s' not found\n", oldName)
		return
	}

	if _, exists := config.Playlists[newName]; exists {
		fmt.Printf("Playlist '%s' already exists\n", newName)
		return
	}

	playlist.Name = newName
	config.Playlists[newName] = playlist
	delete(config.Playlists, oldName)

	// Update current playlist name if it matches
	if config.State.CurrentPlaylist == oldName {
		config.State.CurrentPlaylist = newName
	}

	saveConfig()
	fmt.Printf("Renamed playlist '%s' to '%s'\n", oldName, newName)
}

func handleDelete(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: mfp delete <playlist_name>")
		return
	}

	playlistName := args[0]
	if _, exists := config.Playlists[playlistName]; !exists {
		fmt.Printf("Playlist '%s' not found\n", playlistName)
		return
	}

	// Stop playback if this playlist is currently playing
	if config.State.CurrentPlaylist == playlistName {
		handleStop()
		config.State.CurrentPlaylist = ""
	}

	delete(config.Playlists, playlistName)
	saveConfig()
	fmt.Printf("Deleted playlist '%s'\n", playlistName)
}

func handleStatus() {
	fmt.Println("MFP Status:")
	fmt.Printf("  Volume: %d%%\n", config.State.Volume)
	fmt.Printf("  Shuffle: %s\n", boolToOnOff(config.State.IsShuffle))
	fmt.Printf("  Loop: %s\n", boolToOnOff(config.State.IsLoop))

	if config.State.CurrentPlaylist != "" {
		fmt.Printf("  Current Playlist: %s\n", config.State.CurrentPlaylist)
		playlist := config.Playlists[config.State.CurrentPlaylist]
		if playlist != nil {
			currentIndex := getCurrentSongIndex()
			if currentIndex < len(playlist.Songs) {
				fmt.Printf("  Current Song: %s\n", playlist.Songs[currentIndex].Title)
				fmt.Printf("  Position: %d/%d\n", currentIndex+1, len(playlist.Songs))
			}
		}
		fmt.Printf("  Playing: %s\n", boolToOnOff(config.State.IsPlaying))
	} else {
		fmt.Println("  No playlist loaded")
	}
}

// Helper functions

func boolToOnOff(b bool) string {
	if b {
		return "ON"
	}
	return "OFF"
}

func formatDuration(seconds int) string {
	minutes := seconds / 60
	seconds = seconds % 60
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

func isValidPlaylistURL(url string) bool {
	playlistRegex := regexp.MustCompile(`(?i)(?:youtube\.com/playlist\?list=|youtu\.be/playlist\?list=)([a-zA-Z0-9_-]+)`)
	return playlistRegex.MatchString(url)
}

func extractPlaylistID(url string) string {
	playlistRegex := regexp.MustCompile(`(?i)(?:youtube\.com/playlist\?list=|youtu\.be/playlist\?list=)([a-zA-Z0-9_-]+)`)
	matches := playlistRegex.FindStringSubmatch(url)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func fetchPlaylistSongs(playlistID string) ([]Song, error) {
	// Use yt-dlp to fetch playlist information
	cmd := exec.Command("yt-dlp", "--flat-playlist", "--print", "%(title)s|%(id)s|%(duration_string)s", "--playlist-end", "100", fmt.Sprintf("https://www.youtube.com/playlist?list=%s", playlistID))

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch playlist: %v", err)
	}

	lines := strings.Split(string(output), "\n")
	var songs []Song

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) >= 2 {
			title := parts[0]
			videoID := parts[1]
			duration := "Unknown"
			if len(parts) >= 3 && parts[2] != "NA" {
				duration = parts[2]
			}

			songs = append(songs, Song{
				Title:    title,
				VideoID:  videoID,
				Duration: duration,
				URL:      fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID),
			})
		}
	}

	if len(songs) == 0 {
		return nil, fmt.Errorf("no songs found in playlist")
	}

	return songs, nil
}

func initShuffleOrder() {
	if config.State.CurrentPlaylist == "" {
		return
	}

	playlist := config.Playlists[config.State.CurrentPlaylist]
	if playlist == nil {
		return
	}

	// Create shuffled order
	config.State.ShuffleOrder = make([]int, len(playlist.Songs))
	for i := range config.State.ShuffleOrder {
		config.State.ShuffleOrder[i] = i
	}

	// Shuffle using Fisher-Yates algorithm
	rand.Seed(time.Now().UnixNano())
	for i := len(config.State.ShuffleOrder) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		config.State.ShuffleOrder[i], config.State.ShuffleOrder[j] = config.State.ShuffleOrder[j], config.State.ShuffleOrder[i]
	}

	config.State.ShuffleIndex = 0
}

// Key fixes for the MFP player state management issues

// Improve startPlayback function
func startPlayback() {
	playlist := config.Playlists[config.State.CurrentPlaylist]
	if playlist == nil {
		fmt.Println("Error: Current playlist not found")
		return
	}

	// Set state BEFORE starting mpv
	config.State.IsPlaying = true
	if err := saveConfig(); err != nil {
		fmt.Printf("Error saving state: %v\n", err)
	}

	// Create temporary playlist file for mpv
	playlistFile := filepath.Join(config.DataDir, "current_playlist.m3u")
	if err := createPlaylistFile(playlist, playlistFile); err != nil {
		fmt.Printf("Error creating playlist file: %v\n", err)
		config.State.IsPlaying = false
		saveConfig()
		return
	}

	// Start mpv with the playlist
	if err := startMpv(playlistFile); err != nil {
		fmt.Printf("Error starting mpv: %v\n", err)
		config.State.IsPlaying = false
		saveConfig()
		return
	}

	fmt.Printf("MPV started successfully for playlist: %s\n", config.State.CurrentPlaylist)

	// Start monitoring in background
	go monitorMpv()

	// Don't wait here - let it run in background
	// The Wait() should be handled in the monitor goroutine
}

// Improve handlePlay function
func handlePlay(args []string) {
	if len(args) == 0 {
		// Resume current playlist if available
		if config.State.CurrentPlaylist == "" {
			fmt.Println("No playlist specified. Use: mfp play <playlist_name>")
			return
		}
		fmt.Printf("Resuming playlist: %s\n", config.State.CurrentPlaylist)
	} else {
		// Start new playlist
		playlistName := args[0]
		if _, exists := config.Playlists[playlistName]; !exists {
			fmt.Printf("Playlist '%s' not found\n", playlistName)
			return
		}

		// Stop current playback if any
		if config.State.IsPlaying {
			handleStop()
			time.Sleep(500 * time.Millisecond) // Give time for cleanup
		}

		config.State.CurrentPlaylist = playlistName
		config.State.CurrentSongIndex = 0
		config.State.Position = 0

		// Initialize shuffle order if shuffle is enabled
		if config.State.IsShuffle {
			initShuffleOrder()
		}

		fmt.Printf("Loading playlist: %s\n", playlistName)
	}

	if config.State.IsPlaying {
		fmt.Println("Already playing. Use 'mfp stop' to stop current playback.")
		return
	}

	// Start playback - this should run in background
	go startPlayback()

	// Give it a moment to start, then confirm
	time.Sleep(1 * time.Second)
	if config.State.IsPlaying {
		fmt.Printf("Started playing playlist: %s\n", config.State.CurrentPlaylist)
	} else {
		fmt.Println("Failed to start playback")
	}
}

// Fixed monitorMpv function to properly track current song
func monitorMpv() {
	defer func() {
		config.State.IsPlaying = false
		saveConfig()
		if currentCmd != nil {
			currentCmd = nil
		}
		// Clean up socket file
		os.Remove(config.SocketFile)
	}()

	// Wait for socket to be available
	maxWait := 10 // seconds
	for i := 0; i < maxWait; i++ {
		if _, err := os.Stat(config.SocketFile); err == nil {
			break
		}
		time.Sleep(time.Second)
		if i == maxWait-1 {
			fmt.Println("Error: MPV socket not created, playback may have failed")
			return
		}
	}

	fmt.Println("MPV connection established")
	lastPlaylistPos := -1 // Track the last known position to detect changes

	for {
		if currentCmd == nil {
			break
		}

		// Check if process is still running
		if currentCmd.ProcessState != nil {
			fmt.Println("MPV process ended")
			break
		}

		// Update position
		pos := getMpvPosition()
		if pos >= 0 {
			config.State.Position = pos
		}

		// Update current song index based on mpv's playlist position
		playlistPos := getMpvPlaylistPosition()
		if playlistPos >= 0 && playlistPos != lastPlaylistPos {
			// MPV playlist position changed - update our state
			lastPlaylistPos = playlistPos

			playlist := config.Playlists[config.State.CurrentPlaylist]
			if playlist != nil {
				if config.State.IsShuffle {
					// In shuffle mode, playlistPos is the index in the shuffled order
					if playlistPos < len(config.State.ShuffleOrder) {
						config.State.ShuffleIndex = playlistPos
						config.State.CurrentSongIndex = config.State.ShuffleOrder[playlistPos]
					}
				} else {
					// In normal mode, playlistPos is the direct song index
					if playlistPos < len(playlist.Songs) {
						config.State.CurrentSongIndex = playlistPos
					}
				}

				// Save the updated state
				if err := saveConfig(); err == nil {
					// Optional: Print song change notification
					if playlistPos < len(playlist.Songs) {
						currentIndex := getCurrentSongIndex()
						if currentIndex < len(playlist.Songs) {
							fmt.Printf("Now playing: %s\n", playlist.Songs[currentIndex].Title)
						}
					}
				}
			}
		}

		time.Sleep(1 * time.Second) // Check every second for better responsiveness
	}
}

// Improved getMpvPlaylistPosition with better error handling
func getMpvPlaylistPosition() int {
	if _, err := os.Stat(config.SocketFile); os.IsNotExist(err) {
		return -1
	}

	// Use timeout to prevent hanging
	cmd := exec.Command("timeout", "2s", "sh", "-c",
		fmt.Sprintf(`echo '{"command": ["get_property", "playlist-pos"]}' | socat - %s 2>/dev/null`, config.SocketFile))

	output, err := cmd.Output()
	if err != nil {
		return -1
	}

	var response map[string]interface{}
	if err := json.Unmarshal(output, &response); err != nil {
		return -1
	}

	if data, ok := response["data"].(float64); ok {
		return int(data)
	}

	return -1
}

// Improved getMpvPosition with better error handling
func getMpvPosition() int {
	if _, err := os.Stat(config.SocketFile); os.IsNotExist(err) {
		return -1
	}

	// Use timeout to prevent hanging
	cmd := exec.Command("timeout", "2s", "sh", "-c",
		fmt.Sprintf(`echo '{"command": ["get_property", "time-pos"]}' | socat - %s 2>/dev/null`, config.SocketFile))

	output, err := cmd.Output()
	if err != nil {
		return -1
	}

	var response map[string]interface{}
	if err := json.Unmarshal(output, &response); err != nil {
		return -1
	}

	if data, ok := response["data"].(float64); ok {
		return int(data)
	}

	return -1
}

// Improved the getCurrentSongIndex function for better safety
func getCurrentSongIndex() int {
	if config.State.CurrentPlaylist == "" {
		return 0
	}

	playlist := config.Playlists[config.State.CurrentPlaylist]
	if playlist == nil {
		return 0
	}

	if config.State.IsShuffle {
		if config.State.ShuffleIndex >= 0 && config.State.ShuffleIndex < len(config.State.ShuffleOrder) {
			shuffledIndex := config.State.ShuffleOrder[config.State.ShuffleIndex]
			if shuffledIndex >= 0 && shuffledIndex < len(playlist.Songs) {
				return shuffledIndex
			}
		}
		return 0
	}

	if config.State.CurrentSongIndex >= 0 && config.State.CurrentSongIndex < len(playlist.Songs) {
		return config.State.CurrentSongIndex
	}

	return 0
}

// Improve startMpv function
func startMpv(playlistFile string) error {
	// Clean up old socket
	os.Remove(config.SocketFile)

	startIndex := config.State.CurrentSongIndex
	if config.State.IsShuffle {
		startIndex = config.State.ShuffleIndex
	}

	args := []string{
		"--no-video",
		"--no-terminal", // Run in background
		"--input-ipc-server=" + config.SocketFile,
		"--volume=" + strconv.Itoa(config.State.Volume),
		"--playlist=" + playlistFile,
		"--playlist-start=" + strconv.Itoa(startIndex),
		"--quiet", // Reduce output noise
	}

	if config.State.IsLoop {
		args = append(args, "--loop-playlist=inf")
	}

	currentCmd = exec.Command("mpv", args...)

	// Don't pipe stdout/stderr to avoid blocking
	currentCmd.Stdout = nil
	currentCmd.Stderr = nil

	if err := currentCmd.Start(); err != nil {
		return fmt.Errorf("failed to start mpv: %v", err)
	}

	return nil
}

// Improve handleCurrent function
func handleCurrent() {
	if config.State.CurrentPlaylist == "" {
		fmt.Println("No playlist is currently loaded")
		return
	}

	playlist := config.Playlists[config.State.CurrentPlaylist]
	if playlist == nil {
		fmt.Println("Current playlist not found")
		return
	}

	currentIndex := getCurrentSongIndex()
	if currentIndex >= len(playlist.Songs) || currentIndex < 0 {
		fmt.Println("No current song")
		return
	}

	song := playlist.Songs[currentIndex]
	status := "Paused"
	if config.State.IsPlaying {
		status = "Playing"
	}

	fmt.Printf("Current Song (%s):\n", status)
	fmt.Printf("  Title: %s\n", song.Title)
	fmt.Printf("  Duration: %s\n", song.Duration)
	fmt.Printf("  Position: %d/%d in playlist\n", currentIndex+1, len(playlist.Songs))
	fmt.Printf("  Playlist: %s\n", config.State.CurrentPlaylist)

	// Try to get current position from mpv
	if config.State.IsPlaying {
		if pos := getMpvPosition(); pos >= 0 {
			fmt.Printf("  Time: %s\n", formatDuration(pos))
		}
	}
}

func createPlaylistFile(playlist *Playlist, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	file.WriteString("#EXTM3U\n")

	var songsToWrite []Song
	if config.State.IsShuffle {
		// Write songs in shuffle order
		for _, index := range config.State.ShuffleOrder {
			if index < len(playlist.Songs) {
				songsToWrite = append(songsToWrite, playlist.Songs[index])
			}
		}
	} else {
		songsToWrite = playlist.Songs
	}

	for _, song := range songsToWrite {
		file.WriteString(fmt.Sprintf("#EXTINF:-1,%s\n", song.Title))
		file.WriteString(fmt.Sprintf("%s\n", song.URL))
	}

	return nil
}

func sendMpvCommand(command string) error {
	if _, err := os.Stat(config.SocketFile); os.IsNotExist(err) {
		return fmt.Errorf("mpv socket not found")
	}

	// Parse command into proper JSON format
	var jsonCmd string
	parts := strings.Fields(command)
	if len(parts) == 1 {
		jsonCmd = fmt.Sprintf(`{"command": ["%s"]}`, parts[0])
	} else if len(parts) == 2 {
		jsonCmd = fmt.Sprintf(`{"command": ["%s", "%s"]}`, parts[0], parts[1])
	} else if len(parts) == 3 {
		jsonCmd = fmt.Sprintf(`{"command": ["%s", "%s", "%s"]}`, parts[0], parts[1], parts[2])
	} else {
		jsonCmd = fmt.Sprintf(`{"command": ["%s"]}`, parts[0])
	}

	// Send command via socat
	cmd := exec.Command("sh", "-c", fmt.Sprintf(`echo '%s' | socat - %s`, jsonCmd, config.SocketFile))
	return cmd.Run()
}

func showHelp() {
	fmt.Println("MFP - Music From Playlists")
	fmt.Println("A terminal-based YouTube playlist music player")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  add <name> <url>        Add a YouTube playlist")
	fmt.Println("  play [playlist]         Start/resume playback")
	fmt.Println("  stop                    Stop playback")
	fmt.Println("  next                    Skip to next song")
	fmt.Println("  prev/previous           Go to previous song")
	fmt.Println("  current/now             Show current playing song")
	fmt.Println("  queue [count]           Show playlist queue")
	fmt.Println("  jump <number>           Jump to specific song")
	fmt.Println("  shuffle [on|off]        Toggle/set shuffle mode")
	fmt.Println("  loop [on|off]           Toggle/set loop mode")
	fmt.Println("  volume/vol [up|down|N]  Control volume (0-100)")
	fmt.Println("  seek [+|-]<seconds>     Seek in current song")
	fmt.Println("  list/playlists          List all playlists")
	fmt.Println("  songs <playlist>        List songs in playlist")
	fmt.Println("  rename <old> <new>      Rename a playlist")
	fmt.Println("  delete/remove <name>    Delete a playlist")
	fmt.Println("  status                  Show player status")
	fmt.Println("  help                    Show this help")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  mfp add rock https://www.youtube.com/playlist?list=PLxxx...")
	fmt.Println("  mfp play rock")
	fmt.Println("  mfp volume 80")
	fmt.Println("  mfp shuffle on")
	fmt.Println("  mfp jump 5")
	fmt.Println()
	fmt.Println("Requirements:")
	fmt.Println("  - mpv (media player)")
	fmt.Println("  - yt-dlp (YouTube downloader)")
	fmt.Println("  - socat (socket communication)")
}
