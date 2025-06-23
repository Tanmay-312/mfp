# 🎵 MFP (Music From Playlists)

A complete, production-ready command-line music player that streams YouTube playlists directly in your terminal. Built with Go for high performance and cross-platform compatibility.

## ⚡ Quick Installation

Install and start listening with a single command:

```bash
curl -fsSL https://raw.githubusercontent.com/Tanmay-312/mfp/main/install.sh | bash
```

## 🚀 Features

✅ **Playback Controls**: play, stop, next, previous, jump to any song  
✅ **Queue Management**: view current song, queue display, position tracking  
✅ **Audio Controls**: volume control (0-100%), shuffle, loop modes  
✅ **Playlist Management**: add from YouTube URLs, list, rename, delete  
✅ **Background Operation**: non-blocking playback, control from any terminal  
✅ **Persistent State**: remembers playlists and resumes playback position  
✅ **Smart Shuffle**: Fisher-Yates algorithm for true randomization  
✅ **Cross-Platform**: works on Linux, WSL, and macOS  

## 🎯 Quick Start

```bash
# Add your favorite playlists
mfp add rock "https://www.youtube.com/playlist?list=PLx..."
mfp add chill "https://www.youtube.com/playlist?list=PLy..."

# Start listening
mfp play rock
mfp shuffle on
mfp volume 75

# Control playback
mfp next        # Skip to next song
mfp previous    # Go back to previous song
mfp current     # Show what's currently playing
mfp queue 10    # Show next 10 songs in queue
mfp jump 5      # Jump directly to song #5

# Manage your music
mfp list        # Show all your playlists
mfp songs rock  # Show all songs in 'rock' playlist
mfp stop        # Stop playback
```

## 📋 Complete Command Reference

### Playlist Management

```bash
mfp add <name> <youtube_url>     # Add playlist from YouTube
mfp list                         # Show all playlists
mfp songs <playlist>             # Show songs in playlist
mfp rename <old> <new>           # Rename playlist
mfp delete <playlist>            # Delete playlist
```

### Playback Control

```bash
mfp play <playlist>              # Start playing playlist
mfp stop                         # Stop playback
mfp next                         # Skip to next song
mfp previous                     # Go to previous song
mfp jump <number>                # Jump to specific song number
mfp current                      # Show currently playing song
```

### Audio & Queue

```bash
mfp volume <0-100>               # Set volume percentage
mfp volume up                    # Increase volume by 10%
mfp volume down                  # Decrease volume by 10%
mfp queue [count]                # Show upcoming songs (default: 5)
mfp shuffle <on|off>             # Toggle shuffle mode
mfp loop <on|off>                # Toggle loop mode
```

## 🛠 What the Installer Does

The `install.sh` script automatically:

- Detects your operating system (Linux/WSL/macOS)
- Installs required dependencies (`yt-dlp`, `ffmpeg`)
- Downloads and builds the Go application
- Sets up the `mfp` command globally in your PATH
- Configures everything for immediate use

## 🏗 Architecture & Technology

- **Backend**: Pure Go with standard library (no external Go dependencies)
- **Audio Engine**: `yt-dlp` + `ffplay` for high-quality streaming
- **Storage**: JSON files in `~/.mfp/` directory for playlists and state
- **Concurrency**: Goroutines for smooth background playback
- **Cross-Platform**: Native support for Linux, WSL, and macOS

## 📦 System Requirements

- **OS**: Linux, macOS, or Windows with WSL
- **Dependencies**: `curl`, `bash` (for installation)
- **Runtime**: `ffmpeg`, `yt-dlp` (auto-installed)
- **Internet**: Required for streaming YouTube content

## 🔧 Manual Installation

If you prefer to install manually:

```bash
git clone https://github.com/Tanmay-312/mfp.git
cd mfp
chmod +x install.sh
./install.sh
```

## 🎛 Advanced Features

- **Smart Shuffle**: Uses Fisher-Yates algorithm for truly random playback
- **Persistent State**: Automatically resumes where you left off
- **Queue Preview**: See previous and upcoming songs with configurable count
- **Volume Control**: Granular 0-100% control with quick up/down shortcuts
- **Background Operation**: Music continues playing while you work in other terminals
- **Error Handling**: Graceful recovery from network issues and invalid URLs
- **Signal Handling**: Clean shutdown with Ctrl+C

## 🐛 Troubleshooting

**Installation Issues:**

- Ensure you have `curl` and `bash` installed
- Check internet connection for dependency downloads
- Try manual installation if automated install fails

**Playback Issues:**

- Verify `ffmpeg` and `yt-dlp` are properly installed
- Check if YouTube URLs are accessible
- Ensure you have sufficient disk space in `~/.mfp/`

**Command Not Found:**

- Restart your terminal after installation
- Check if `~/.local/bin` is in your PATH
- Try running `source ~/.bashrc` or `source ~/.zshrc`

## 📁 Project Structure

```bash
mfp/
├── main.go          # Core application
├── install.sh       # Automated installer
├── go.mod          # Go module definition
├── README.md       # This file
├── LICENSE         # MIT License
└── .gitignore      # Git ignore rules
```

## 🤝 Contributing

Contributions are welcome! Please feel free to:

1. Fork the repository
2. Create a feature branch
3. Submit a pull request

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🔗 Links

- **Repository**: <https://github.com/Tanmay-312/mfp>
- **Issues**: <https://github.com/Tanmay-312/mfp/issues>
- **Releases**: <https://github.com/Tanmay-312/mfp/releases>

---

**🎵 Start listening now:**

```bash
`curl -fsSL https://raw.githubusercontent.com/Tanmay-312/mfp/main/install.sh | bash`
```
