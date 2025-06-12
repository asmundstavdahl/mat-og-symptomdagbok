package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Rough estimate of tokens per character for typical code
// This is a simplification - actual tokenization varies by model
const tokensPerChar = 0.25

// FileTokenInfo stores token information for a file
type FileTokenInfo struct {
	Path      string
	Size      int64
	TokenEst  int64
	Extension string
}

func main() {
	// Get current directory
	dir, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error getting current directory: %v\n", err)
		return
	}

	// Collect file information
	var files []FileTokenInfo
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and hidden files
		if info.IsDir() || strings.HasPrefix(filepath.Base(path), ".") {
			return nil
		}

		// Skip binary files and certain directories
		if shouldSkip(path) {
			return nil
		}

		// Get file extension
		ext := filepath.Ext(path)
		if ext == "" {
			ext = "[no extension]"
		}

		// Calculate relative path
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			relPath = path
		}

		// Get file size
		size := info.Size()

		// Estimate tokens (very rough approximation)
		tokenEst := int64(float64(size) * tokensPerChar)

		files = append(files, FileTokenInfo{
			Path:      relPath,
			Size:      size,
			TokenEst:  tokenEst,
			Extension: ext,
		})

		return nil
	})

	if err != nil {
		fmt.Printf("Error walking directory: %v\n", err)
		return
	}

	// Sort by token count (descending)
	sort.Slice(files, func(i, j int) bool {
		return files[i].TokenEst > files[j].TokenEst
	})

	// Print summary
	fmt.Println("Token Usage Analysis for LLM Context")
	fmt.Println("====================================")
	fmt.Println()

	var totalSize int64
	var totalTokens int64
	fmt.Printf("%-50s | %-10s | %-10s | %s\n", "File", "Size (B)", "Est. Tokens", "Extension")
	fmt.Println(strings.Repeat("-", 90))

	for _, file := range files {
		fmt.Printf("%-50s | %-10d | %-10d | %s\n", 
			truncatePath(file.Path, 50), 
			file.Size, 
			file.TokenEst,
			file.Extension)
		
		totalSize += file.Size
		totalTokens += file.TokenEst
	}

	fmt.Println(strings.Repeat("-", 90))
	fmt.Printf("%-50s | %-10d | %-10d |\n", "TOTAL", totalSize, totalTokens)
	fmt.Println()

	// Extension summary
	extMap := make(map[string]int64)
	for _, file := range files {
		extMap[file.Extension] += file.TokenEst
	}

	// Convert map to slice for sorting
	type ExtInfo struct {
		Ext   string
		Tokens int64
	}
	var extInfos []ExtInfo
	for ext, tokens := range extMap {
		extInfos = append(extInfos, ExtInfo{ext, tokens})
	}

	// Sort by token count
	sort.Slice(extInfos, func(i, j int) bool {
		return extInfos[i].Tokens > extInfos[j].Tokens
	})

	fmt.Println("Token Usage by File Extension")
	fmt.Println("============================")
	fmt.Printf("%-15s | %-10s | %s\n", "Extension", "Est. Tokens", "% of Total")
	fmt.Println(strings.Repeat("-", 50))

	for _, info := range extInfos {
		percentage := float64(info.Tokens) / float64(totalTokens) * 100
		fmt.Printf("%-15s | %-10d | %.2f%%\n", info.Ext, info.Tokens, percentage)
	}
}

// shouldSkip returns true if the file should be skipped
func shouldSkip(path string) bool {
	// Skip binary files and certain directories
	lowerPath := strings.ToLower(path)
	
	// Skip binary and generated files
	skipExts := []string{
		".exe", ".dll", ".so", ".dylib", ".bin", ".obj", ".o",
		".a", ".lib", ".pyc", ".pyo", ".class", ".jar", ".war",
		".ear", ".zip", ".tar", ".gz", ".rar", ".7z", ".db",
		".sqlite", ".sqlite3", ".pdf", ".doc", ".docx", ".ppt",
		".pptx", ".xls", ".xlsx", ".jpg", ".jpeg", ".png", ".gif",
		".bmp", ".ico", ".svg", ".mp3", ".mp4", ".avi", ".mov",
		".flv", ".wmv", ".wma", ".ogg", ".wav", ".flac",
	}
	
	for _, ext := range skipExts {
		if strings.HasSuffix(lowerPath, ext) {
			return true
		}
	}
	
	// Skip certain directories
	skipDirs := []string{
		"node_modules", "vendor", "dist", "build", "bin",
		".git", ".svn", ".hg", ".idea", ".vscode",
	}
	
	for _, dir := range skipDirs {
		if strings.Contains(lowerPath, "/"+dir+"/") || strings.HasSuffix(lowerPath, "/"+dir) {
			return true
		}
	}
	
	return false
}

// truncatePath truncates a path to the specified length
func truncatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	
	// Try to keep the filename and some of the path
	fileName := filepath.Base(path)
	dirName := filepath.Dir(path)
	
	if len(fileName) >= maxLen-3 {
		// If filename itself is too long, truncate it
		return "..." + fileName[len(fileName)-(maxLen-3):]
	}
	
	// Keep the filename and truncate the directory part
	return "..." + dirName[len(dirName)-(maxLen-len(fileName)-3):] + "/" + fileName
}
