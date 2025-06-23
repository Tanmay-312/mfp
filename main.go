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
}

// Config holds application configuration
type Config struct {
	DataDir   string
	StateFile string
	Playlists map[string]*Playlist
	State     *PlayerState
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
	playlistsFile := filepath.Join(dataDir, "playlists.json")

	config := &Config{
		DataDir:   dataDir,
		StateFile: stateFile,
		Playlists: make(map[string]*Playlist),
		State: &PlayerState{
			Volume:           70,
			CurrentSongIndex: 0,
			ShuffleOrder:     []int{},
			ShuffleIndex:     0,
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
		if currentCmd != nil && currentCmd.Process != nil {
			currentCmd.Process.Kill()
		}
		config.State.IsPlaying = false
		saveConfig()
		os.Exit(0)
	}()
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

func handlePlay(args []string) {
	if len(args) == 0 {
		// Resume current playlist if available
		if config.State.CurrentPlaylist == "" {
			fmt.Println("No playlist specified. Use: mfp play <playlist_name>")
			return
		}
	} else {
		// Start new playlist
		playlistName := args[0]
		if _, exists := config.Playlists[playlistName]; !exists {
			fmt.Printf("Playlist '%s' not found\n", playlistName)
			return
		}
		config.State.CurrentPlaylist = playlistName
		config.State.CurrentSongIndex = 0

		// Initialize shuffle order if shuffle is enabled
		if config.State.IsShuffle {
			initShuffleOrder()
		}
	}

	if config.State.IsPlaying {
		fmt.Println("Already playing. Use 'mfp stop' to stop current playback.")
		return
	}

	go startPlayback()
	fmt.Printf("Started playing playlist: %s\n", config.State.CurrentPlaylist)
}

func handleStop() {
	if currentCmd != nil && currentCmd.Process != nil {
		currentCmd.Process.Kill()
		currentCmd = nil
	}
	config.State.IsPlaying = false
	saveConfig()
	fmt.Println("Playback stopped")
}

func handleNext() {
	if !config.State.IsPlaying {
		fmt.Println("No music is currently playing")
		return
	}

	skipChannel <- true
	fmt.Println("Skipping to next song...")
}

func handlePrevious() {
	if !config.State.IsPlaying {
		fmt.Println("No music is currently playing")
		return
	}

	// Go to previous song
	if config.State.IsShuffle {
		if config.State.ShuffleIndex > 0 {
			config.State.ShuffleIndex--
		}
	} else {
		if config.State.CurrentSongIndex > 0 {
			config.State.CurrentSongIndex--
		}
	}

	skipChannel <- true
	fmt.Println("Going to previous song...")
}

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
	if currentIndex >= len(playlist.Songs) {
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
		skipChannel <- true
	}

	fmt.Printf("Jumped to song %d: %s\n", songNum, playlist.Songs[targetIndex].Title)
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
		fmt.Println("Shuffle: ON")
	} else {
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
		fmt.Println("Loop: ON")
	} else {
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

	fmt.Printf("Volume set to: %d%%\n", config.State.Volume)
	saveConfig()
}

func handleSeek(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: mfp seek [+|-]<seconds>")
		return
	}

	// This is a placeholder - actual seeking would require more complex mpv integration
	fmt.Printf("Seek functionality not yet implemented with basic audio playback\n")
	fmt.Printf("Requested: %s seconds\n", args[0])
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

func getCurrentSongIndex() int {
	if config.State.IsShuffle {
		if config.State.ShuffleIndex < len(config.State.ShuffleOrder) {
			return config.State.ShuffleOrder[config.State.ShuffleIndex]
		}
		return 0
	}
	return config.State.CurrentSongIndex
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

func startPlayback() {
	config.State.IsPlaying = true
	saveConfig()

	for {
		playlist := config.Playlists[config.State.CurrentPlaylist]
		if playlist == nil {
			break
		}

		currentIndex := getCurrentSongIndex()
		if currentIndex >= len(playlist.Songs) {
			break
		}

		song := playlist.Songs[currentIndex]
		fmt.Printf("Now playing: %s\n", song.Title)

		// Play the song using yt-dlp and a media player
		if err := playSong(song); err != nil {
			fmt.Printf("Error playing song: %v\n", err)
		}

		// Check if we should continue to next song
		if !shouldContinuePlayback() {
			break
		}

		// Move to next song
		moveToNextSong()
	}

	config.State.IsPlaying = false
	saveConfig()
}

func playSong(song Song) error {
	// Get audio stream URL using yt-dlp
	cmd := exec.Command("yt-dlp", "-f", "bestaudio", "--get-url", song.URL)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get audio URL: %v", err)
	}

	audioURL := strings.TrimSpace(string(output))

	// Play using ffplay (part of ffmpeg) with volume control
	volumeFilter := fmt.Sprintf("volume=%f", float64(config.State.Volume)/100.0)
	currentCmd = exec.Command("ffplay", "-nodisp", "-autoexit", "-af", volumeFilter, audioURL)

	// Start the command
	if err := currentCmd.Start(); err != nil {
		return fmt.Errorf("failed to start audio player: %v", err)
	}

	// Wait for completion or skip signal
	done := make(chan error)
	go func() {
		done <- currentCmd.Wait()
	}()

	select {
	case <-skipChannel:
		// Skip requested
		if currentCmd.Process != nil {
			currentCmd.Process.Kill()
		}
		return nil
	case <-quitChannel:
		// Quit requested
		if currentCmd.Process != nil {
			currentCmd.Process.Kill()
		}
		return fmt.Errorf("playback stopped")
	case err := <-done:
		// Song finished naturally
		currentCmd = nil
		return err
	}
}

func shouldContinuePlayback() bool {
	playlist := config.Playlists[config.State.CurrentPlaylist]
	if playlist == nil {
		return false
	}

	if config.State.IsShuffle {
		// Check if we've reached the end of the shuffle order
		if config.State.ShuffleIndex >= len(config.State.ShuffleOrder)-1 {
			if config.State.IsLoop {
				// Restart shuffle
				initShuffleOrder()
				return true
			}
			return false
		}
	} else {
		// Check if we've reached the end of the playlist
		if config.State.CurrentSongIndex >= len(playlist.Songs)-1 {
			if config.State.IsLoop {
				// Restart playlist
				config.State.CurrentSongIndex = 0
				return true
			}
			return false
		}
	}

	return true
}

func moveToNextSong() {
	if config.State.IsShuffle {
		config.State.ShuffleIndex++
	} else {
		config.State.CurrentSongIndex++
	}
	saveConfig()
}

func showHelp() {
	fmt.Println(`MFP - Music From Playlists
A command-line YouTube playlist music player

PLAYLIST MANAGEMENT:
  mfp add <name> <url>         Add YouTube playlist
  mfp list                     Show all playlists
  mfp songs <playlist>         Show songs in playlist
  mfp rename <old> <new>       Rename playlist
  mfp delete <playlist>        Delete playlist

PLAYBACK CONTROLS:
  mfp play [playlist]          Start/resume playback
  mfp stop                     Stop playback
  mfp next                     Skip to next song
  mfp prev                     Go to previous song
  mfp jump <number>            Jump to specific song

QUEUE & INFO:
  mfp current                  Show currently playing song
  mfp queue [count]            Show queue (default: 5 songs each way)
  mfp status                   Show player status

SETTINGS:
  mfp shuffle [on|off]         Toggle/set shuffle mode
  mfp loop [on|off]            Toggle/set loop mode
  mfp volume [up|down|0-100]   Control volume
  mfp seek [+|-]<seconds>      Seek forward/backward (future feature)

EXAMPLES:
  mfp add mymusic "https://www.youtube.com/playlist?list=PLx..."
  mfp play mymusic
  mfp volume 80
  mfp shuffle on
  mfp queue 10`)
}
