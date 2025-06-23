#!/bin/bash

# MFP (Music From Playlists) Installation Script
# This script installs all prerequisites, builds the app, and sets up the PATH

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Detect OS
detect_os() {
    if [[ "$OSTYPE" == "linux-gnu"* ]]; then
        if grep -q Microsoft /proc/version; then
            echo "wsl"
        else
            echo "linux"
        fi
    elif [[ "$OSTYPE" == "darwin"* ]]; then
        echo "macos"
    else
        echo "unknown"
    fi
}

# Install dependencies based on OS
install_dependencies() {
    local os=$(detect_os)
    
    print_status "Detected OS: $os"
    
    case $os in
        "linux"|"wsl")
            print_status "Installing dependencies for Linux/WSL..."
            
            # Update package lists
            if command_exists apt-get; then
                sudo apt-get update -qq
                
                # Install ffmpeg
                if ! command_exists ffplay; then
                    print_status "Installing ffmpeg..."
                    sudo apt-get install -y ffmpeg
                fi
                
                # Install python3 and pip if needed for yt-dlp
                if ! command_exists python3; then
                    print_status "Installing python3..."
                    sudo apt-get install -y python3 python3-pip
                fi
                
            elif command_exists yum; then
                # RHEL/CentOS/Fedora
                if ! command_exists ffplay; then
                    print_status "Installing ffmpeg..."
                    sudo yum install -y ffmpeg
                fi
                
                if ! command_exists python3; then
                    print_status "Installing python3..."
                    sudo yum install -y python3 python3-pip
                fi
                
            elif command_exists pacman; then
                # Arch Linux
                if ! command_exists ffplay; then
                    print_status "Installing ffmpeg..."
                    sudo pacman -S --noconfirm ffmpeg
                fi
                
                if ! command_exists python3; then
                    print_status "Installing python3..."
                    sudo pacman -S --noconfirm python python-pip
                fi
            fi
            ;;
            
        "macos")
            print_status "Installing dependencies for macOS..."
            
            if ! command_exists brew; then
                print_error "Homebrew is required but not installed."
                print_status "Please install Homebrew first: https://brew.sh/"
                exit 1
            fi
            
            if ! command_exists ffplay; then
                print_status "Installing ffmpeg..."
                brew install ffmpeg
            fi
            
            if ! command_exists python3; then
                print_status "Installing python3..."
                brew install python3
            fi
            ;;
            
        *)
            print_error "Unsupported operating system: $os"
            print_status "Please install the following manually:"
            print_status "- Go 1.19+"
            print_status "- ffmpeg (for ffplay command)"
            print_status "- yt-dlp (pip install yt-dlp)"
            exit 1
            ;;
    esac
}

# Install yt-dlp
install_yt_dlp() {
    if ! command_exists yt-dlp; then
        print_status "Installing yt-dlp..."
        
        # Try pip3 first, then pip
        if command_exists pip3; then
            pip3 install --user yt-dlp
        elif command_exists pip; then
            pip install --user yt-dlp
        else
            print_error "pip is required to install yt-dlp"
            exit 1
        fi
        
        # Add user bin to PATH if not already there
        if [[ ":$PATH:" != *":$HOME/.local/bin:"* ]]; then
            export PATH="$HOME/.local/bin:$PATH"
        fi
    else
        print_success "yt-dlp is already installed"
    fi
}

# Check Go installation
check_go() {
    if ! command_exists go; then
        print_error "Go is not installed!"
        print_status "Please install Go 1.19 or later from: https://golang.org/dl/"
        
        local os=$(detect_os)
        case $os in
            "linux"|"wsl")
                print_status "On Ubuntu/Debian: sudo apt install golang-go"
                print_status "On RHEL/CentOS: sudo yum install golang"
                ;;
            "macos")
                print_status "On macOS: brew install go"
                ;;
        esac
        exit 1
    fi
    
    # Check Go version
    local go_version=$(go version | grep -o 'go[0-9]\+\.[0-9]\+' | sed 's/go//')
    local major=$(echo $go_version | cut -d. -f1)
    local minor=$(echo $go_version | cut -d. -f2)
    
    if [ "$major" -lt 1 ] || ([ "$major" -eq 1 ] && [ "$minor" -lt 19 ]); then
        print_error "Go version $go_version is too old. Please install Go 1.19 or later."
        exit 1
    fi
    
    print_success "Go $go_version is installed"
}

# Build the application
build_app() {
    print_status "Building MFP..."
    
    # Initialize go module if not exists
    if [ ! -f "go.mod" ]; then
        go mod init mfp
    fi
    
    # Build the application
    go build -o mfp main.go
    
    if [ $? -eq 0 ]; then
        print_success "MFP built successfully"
    else
        print_error "Failed to build MFP"
        exit 1
    fi
}

# Install the binary
install_binary() {
    print_status "Installing MFP binary..."
    
    # Create local bin directory if it doesn't exist
    mkdir -p "$HOME/.local/bin"
    
    # Copy binary to local bin
    cp mfp "$HOME/.local/bin/mfp"
    chmod +x "$HOME/.local/bin/mfp"
    
    print_success "MFP installed to $HOME/.local/bin/mfp"
}

# Setup PATH
setup_path() {
    local shell_rc=""
    local current_shell=$(basename "$SHELL")
    
    case $current_shell in
        "bash")
            shell_rc="$HOME/.bashrc"
            ;;
        "zsh")
            shell_rc="$HOME/.zshrc"
            ;;
        "fish")
            shell_rc="$HOME/.config/fish/config.fish"
            ;;
        *)
            shell_rc="$HOME/.profile"
            ;;
    esac
    
    # Check if PATH already includes ~/.local/bin
    if [[ ":$PATH:" != *":$HOME/.local/bin:"* ]]; then
        print_status "Adding $HOME/.local/bin to PATH in $shell_rc"
        
        if [ "$current_shell" = "fish" ]; then
            echo "set -gx PATH \$HOME/.local/bin \$PATH" >> "$shell_rc"
        else
            echo 'export PATH="$HOME/.local/bin:$PATH"' >> "$shell_rc"
        fi
        
        # Also export for current session
        export PATH="$HOME/.local/bin:$PATH"
        
        print_success "PATH updated in $shell_rc"
        print_warning "Please run 'source $shell_rc' or restart your terminal"
    else
        print_success "PATH already includes $HOME/.local/bin"
    fi
}

# Verify installation
verify_installation() {
    print_status "Verifying installation..."
    
    # Check if mfp command works
    if command_exists mfp; then
        print_success "MFP command is available"
        
        # Test basic functionality
        mfp help >/dev/null 2>&1
        if [ $? -eq 0 ]; then
            print_success "MFP is working correctly"
        else
            print_warning "MFP command exists but may have issues"
        fi
    else
        print_error "MFP command not found in PATH"
        print_status "You may need to restart your terminal or run:"
        print_status "source ~/.bashrc  # or ~/.zshrc"
    fi
    
    # Check dependencies
    if command_exists yt-dlp; then
        print_success "yt-dlp is available"
    else
        print_error "yt-dlp not found in PATH"
    fi
    
    if command_exists ffplay; then
        print_success "ffplay is available"
    else
        print_error "ffplay not found in PATH"
    fi
}

# Main installation function
main() {
    echo -e "${BLUE}"
    echo "╔═══════════════════════════════════════╗"
    echo "║           MFP Installer               ║"
    echo "║     Music From Playlists v1.0        ║"
    echo "╚═══════════════════════════════════════╝"
    echo -e "${NC}"
    
    print_status "Starting MFP installation..."
    
    # Check if we're in the right directory
    if [ ! -f "main.go" ]; then
        print_error "main.go not found. Please run this script from the MFP project directory."
        exit 1
    fi
    
    # Step 1: Check Go installation
    check_go
    
    # Step 2: Install system dependencies
    install_dependencies
    
    # Step 3: Install yt-dlp
    install_yt_dlp
    
    # Step 4: Build the application
    build_app
    
    # Step 5: Install binary
    install_binary
    
    # Step 6: Setup PATH
    setup_path
    
    # Step 7: Verify installation
    verify_installation
    
    echo -e "${GREEN}"
    echo "╔═══════════════════════════════════════╗"
    echo "║        Installation Complete!         ║"
    echo "╚═══════════════════════════════════════╝"
    echo -e "${NC}"
    
    print_success "MFP has been installed successfully!"
    echo ""
    print_status "Quick start:"
    echo "  mfp add myplaylist \"https://www.youtube.com/playlist?list=...\"" 
    echo "  mfp play myplaylist"
    echo "  mfp help"
    echo ""
    print_status "If 'mfp' command is not found, try:"
    echo "  source ~/.bashrc"
    echo "  # or restart your terminal"
    echo ""
    print_status "Enjoy your music! 🎵"
}

# Run main function
main "$@"