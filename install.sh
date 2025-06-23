#!/bin/bash

# MFP (Music From Playlists) Complete Installation Script
# This script downloads the repo, installs all prerequisites, builds the app, and sets up the PATH

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
REPO_URL="https://github.com/Tanmay-312/mfp.git"
INSTALL_DIR="$HOME/.mfp-install"
GO_VERSION="1.21.5"

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
        if grep -q Microsoft /proc/version 2>/dev/null; then
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

# Detect architecture
detect_arch() {
    local arch=$(uname -m)
    case $arch in
        x86_64)
            echo "amd64"
            ;;
        aarch64|arm64)
            echo "arm64"
            ;;
        armv6l)
            echo "armv6l"
            ;;
        armv7l)
            echo "armv7l"
            ;;
        *)
            echo "unknown"
            ;;
    esac
}

# Install Go if not present or version is too old
install_go() {
    local need_install=false
    
    if ! command_exists go; then
        print_status "Go is not installed. Installing Go $GO_VERSION..."
        need_install=true
    else
        # Check Go version
        local go_version=$(go version | grep -o 'go[0-9]\+\.[0-9]\+\.[0-9]\+' | sed 's/go//')
        local current_major=$(echo $go_version | cut -d. -f1)
        local current_minor=$(echo $go_version | cut -d. -f2)
        
        if [ "$current_major" -lt 1 ] || ([ "$current_major" -eq 1 ] && [ "$current_minor" -lt 19 ]); then
            print_status "Go version $go_version is too old. Installing Go $GO_VERSION..."
            need_install=true
        else
            print_success "Go $go_version is already installed"
            return 0
        fi
    fi
    
    if [ "$need_install" = true ]; then
        local os=$(detect_os)
        local arch=$(detect_arch)
        
        case $os in
            "linux"|"wsl")
                case $arch in
                    "amd64")
                        local go_archive="go${GO_VERSION}.linux-amd64.tar.gz"
                        ;;
                    "arm64")
                        local go_archive="go${GO_VERSION}.linux-arm64.tar.gz"
                        ;;
                    "armv6l")
                        local go_archive="go${GO_VERSION}.linux-armv6l.tar.gz"
                        ;;
                    *)
                        print_error "Unsupported architecture: $arch"
                        print_status "Please install Go manually from https://golang.org/dl/"
                        exit 1
                        ;;
                esac
                ;;
            "macos")
                case $arch in
                    "amd64")
                        local go_archive="go${GO_VERSION}.darwin-amd64.tar.gz"
                        ;;
                    "arm64")
                        local go_archive="go${GO_VERSION}.darwin-arm64.tar.gz"
                        ;;
                    *)
                        print_error "Unsupported architecture: $arch"
                        exit 1
                        ;;
                esac
                ;;
            *)
                print_error "Unsupported operating system: $os"
                exit 1
                ;;
        esac
        
        # Download and install Go
        local go_url="https://golang.org/dl/${go_archive}"
        local temp_dir=$(mktemp -d)
        
        print_status "Downloading Go from $go_url..."
        if command_exists curl; then
            curl -fsSL "$go_url" -o "$temp_dir/$go_archive"
        elif command_exists wget; then
            wget -q "$go_url" -O "$temp_dir/$go_archive"
        else
            print_error "curl or wget is required to download Go"
            exit 1
        fi
        
        print_status "Installing Go to $HOME/.local/go..."
        mkdir -p "$HOME/.local"
        rm -rf "$HOME/.local/go"
        tar -C "$HOME/.local" -xzf "$temp_dir/$go_archive"
        
        # Add Go to PATH
        export PATH="$HOME/.local/go/bin:$PATH"
        
        # Clean up
        rm -rf "$temp_dir"
        
        print_success "Go $GO_VERSION installed successfully"
    fi
}

# Download repository
download_repo() {
    print_status "Downloading MFP repository..."
    
    # Remove existing installation directory
    if [ -d "$INSTALL_DIR" ]; then
        rm -rf "$INSTALL_DIR"
    fi
    
    # Clone repository
    if command_exists git; then
        git clone "$REPO_URL" "$INSTALL_DIR"
    else
        print_error "git is required to download the repository"
        print_status "Installing git..."
        
        local os=$(detect_os)
        case $os in
            "linux"|"wsl")
                if command_exists apt-get; then
                    sudo apt-get update -qq && sudo apt-get install -y git
                elif command_exists yum; then
                    sudo yum install -y git
                elif command_exists pacman; then
                    sudo pacman -S --noconfirm git
                else
                    print_error "Cannot install git automatically. Please install git manually."
                    exit 1
                fi
                ;;
            "macos")
                if command_exists brew; then
                    brew install git
                else
                    print_error "Please install git manually or install Homebrew first"
                    exit 1
                fi
                ;;
            *)
                print_error "Please install git manually"
                exit 1
                ;;
        esac
        
        # Try cloning again
        git clone "$REPO_URL" "$INSTALL_DIR"
    fi
    
    print_success "Repository downloaded to $INSTALL_DIR"
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
            else
                print_error "No supported package manager found (apt-get, yum, pacman)"
                print_status "Please install ffmpeg and python3 manually"
                exit 1
            fi
            ;;
            
        "macos")
            print_status "Installing dependencies for macOS..."
            
            # Install Homebrew if not present
            if ! command_exists brew; then
                print_status "Installing Homebrew..."
                /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
                
                # Add Homebrew to PATH
                if [[ -f "/opt/homebrew/bin/brew" ]]; then
                    eval "$(/opt/homebrew/bin/brew shellenv)"
                elif [[ -f "/usr/local/bin/brew" ]]; then
                    eval "$(/usr/local/bin/brew shellenv)"
                fi
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
        
        print_success "yt-dlp installed successfully"
    else
        print_success "yt-dlp is already installed"
    fi
}

# Build the application
build_app() {
    print_status "Building MFP..."
    
    cd "$INSTALL_DIR"
    
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
    cp "$INSTALL_DIR/mfp" "$HOME/.local/bin/mfp"
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
    
    # Add Go to PATH
    local go_path_added=false
    if [[ ":$PATH:" != *":$HOME/.local/go/bin:"* ]] && [ -d "$HOME/.local/go/bin" ]; then
        print_status "Adding Go to PATH in $shell_rc"
        
        if [ "$current_shell" = "fish" ]; then
            echo "set -gx PATH \$HOME/.local/go/bin \$PATH" >> "$shell_rc"
        else
            echo 'export PATH="$HOME/.local/go/bin:$PATH"' >> "$shell_rc"
        fi
        go_path_added=true
    fi
    
    # Add ~/.local/bin to PATH
    local local_bin_added=false
    if [[ ":$PATH:" != *":$HOME/.local/bin:"* ]]; then
        print_status "Adding $HOME/.local/bin to PATH in $shell_rc"
        
        if [ "$current_shell" = "fish" ]; then
            echo "set -gx PATH \$HOME/.local/bin \$PATH" >> "$shell_rc"
        else
            echo 'export PATH="$HOME/.local/bin:$PATH"' >> "$shell_rc"
        fi
        local_bin_added=true
        
        # Also export for current session
        export PATH="$HOME/.local/bin:$PATH"
    fi
    
    if [ "$go_path_added" = true ] || [ "$local_bin_added" = true ]; then
        print_success "PATH updated in $shell_rc"
        print_warning "Please run 'source $shell_rc' or restart your terminal"
    else
        print_success "PATH is already configured correctly"
    fi
}

# Verify installation
verify_installation() {
    print_status "Verifying installation..."
    
    # Check if mfp command works
    if command_exists mfp; then
        print_success "MFP command is available"
    else
        print_error "MFP command not found in PATH"
        print_status "You may need to restart your terminal or run:"
        print_status "source ~/.bashrc  # or ~/.zshrc"
        return 1
    fi
    
    # Check dependencies
    if command_exists yt-dlp; then
        print_success "yt-dlp is available"
    else
        print_error "yt-dlp not found in PATH"
        return 1
    fi
    
    if command_exists ffplay; then
        print_success "ffplay is available"
    else
        print_error "ffplay not found in PATH"
        return 1
    fi
    
    return 0
}

# Cleanup function
cleanup() {
    if [ -d "$INSTALL_DIR" ]; then
        print_status "Cleaning up temporary files..."
        rm -rf "$INSTALL_DIR"
    fi
}

# Main installation function
main() {
    echo -e "${BLUE}"
    echo "╔═══════════════════════════════════════╗"
    echo "║           MFP Installer               ║"
    echo "║     Music From Playlists v1.0        ║"
    echo "║         Complete Installation         ║"
    echo "╚═══════════════════════════════════════╝"
    echo -e "${NC}"
    
    print_status "Starting complete MFP installation..."
    print_status "This installer will handle everything automatically!"
    echo ""
    
    # Set up cleanup on exit
    trap cleanup EXIT
    
    # Step 1: Download repository
    download_repo
    
    # Step 2: Install Go if needed
    install_go
    
    # Step 3: Install system dependencies
    install_dependencies
    
    # Step 4: Install yt-dlp
    install_yt_dlp
    
    # Step 5: Build the application
    build_app
    
    # Step 6: Install binary
    install_binary
    
    # Step 7: Setup PATH
    setup_path
    
    # Step 8: Verify installation
    if verify_installation; then
        echo ""
        echo -e "${GREEN}"
        echo "╔═══════════════════════════════════════╗"
        echo "║        Installation Complete!         ║"
        echo "╚═══════════════════════════════════════╝"
        echo -e "${NC}"
        
        print_success "MFP has been installed successfully!"
        echo ""
        print_status "🎵 Quick start guide:"
        echo "  mfp add myplaylist \"https://www.youtube.com/playlist?list=...\""
        echo "  mfp play myplaylist"
        echo "  mfp help"
        echo ""
        print_status "📚 For more commands, run: mfp help"
        echo ""
        print_status "🔄 If 'mfp' command is not found, try:"
        echo "  source ~/.bashrc  # or ~/.zshrc"
        echo "  # or restart your terminal"
        echo ""
        print_status "🎶 Enjoy your music!"
    else
        print_error "Installation completed but verification failed"
        print_status "You may need to restart your terminal or manually configure PATH"
        exit 1
    fi
}

# Run main function
main "$@"