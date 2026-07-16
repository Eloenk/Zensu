package dl

import (
	"archive/zip"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"zensu/internal/logger"
)

const (
	ffmpegWinURL   = "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip"
	ffmpegLinuxURL = "https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-amd64-static.tar.xz"
)

func isFfmpegCallable(path string) bool {
	cmd := exec.Command(path, "-version")
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	setHideWindow(cmd.SysProcAttr)
	err := cmd.Run()
	return err == nil
}

var (
	ffmpegOnce sync.Once
	ffmpegErr  error
)

func logInfo(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	fmt.Println("  [INFO] " + msg)
	logger.Infof("FFMPEG_SETUP", "%s", msg)
}

func logError(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	fmt.Println("  [ERROR] " + msg)
	logger.Errorf("FFMPEG_SETUP", "%s", msg)
}

func EnsureFFmpegOnce() error {
	ffmpegOnce.Do(func() {
		ffmpegErr = EnsureFFmpeg()
	})
	return ffmpegErr
}

func fetchRemoteChecksum(urlStr string) (string, error) {
	resp, err := http.Get(urlStr)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bad status: %s", resp.Status)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	fields := strings.Fields(string(bodyBytes))
	if len(fields) == 0 {
		return "", fmt.Errorf("empty checksum file")
	}

	return strings.ToLower(fields[0]), nil
}

func fetchBtbNChecksum() (string, error) {
	resp, err := http.Get("https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/checksums.sha256")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bad status: %s", resp.Status)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(bodyBytes), "\n")
	for _, line := range lines {
		if strings.Contains(line, "ffmpeg-master-latest-win64-gpl.zip") {
			fields := strings.Fields(line)
			if len(fields) > 0 {
				return strings.ToLower(fields[0]), nil
			}
		}
	}
	return "", fmt.Errorf("checksum for BtbN build not found in checksums.sha256")
}

func verifyFileHash(filePath, expectedHash string, useSHA256 bool) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	var computed []byte
	if useSHA256 {
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return err
		}
		computed = h.Sum(nil)
	} else {
		h := md5.New()
		if _, err := io.Copy(h, f); err != nil {
			return err
		}
		computed = h.Sum(nil)
	}

	computedHex := hex.EncodeToString(computed)
	if computedHex != expectedHash {
		return fmt.Errorf("hash mismatch: expected %s, got %s", expectedHash, computedHex)
	}

	return nil
}

func EnsureFFmpeg() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exeDir := filepath.Dir(exe)
	binDir := filepath.Join(exeDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return err
	}

	binaryName := "ffmpeg"
	if runtime.GOOS == "windows" {
		binaryName = "ffmpeg.exe"
	}
	localPath := filepath.Join(binDir, binaryName)

	if isFfmpegCallable(localPath) {
		return nil
	}

	logInfo("FFmpeg not found. Downloading static build...")

	if runtime.GOOS == "windows" {
		tempZip := filepath.Join(binDir, "ffmpeg-temp.zip")
		
		// Attempt 1: Gyan.dev
		logInfo("Attempting to download FFmpeg from Gyan.dev...")
		downloadSuccess := false
		
		checksumURL := ffmpegWinURL + ".sha256"
		logInfo("Fetching FFmpeg SHA256 checksum from Gyan.dev...")
		expectedHash, err := fetchRemoteChecksum(checksumURL)
		if err == nil {
			if err := downloadFFmpeg(ffmpegWinURL, tempZip); err == nil {
				logInfo("Verifying download integrity...")
				if err := verifyFileHash(tempZip, expectedHash, true); err == nil {
					downloadSuccess = true
				} else {
					logError("Gyan.dev checksum verification failed: %v", err)
				}
			} else {
				logError("Failed to download FFmpeg from Gyan.dev: %v", err)
			}
		} else {
			logError("Failed to fetch Gyan.dev checksum: %v", err)
		}
		
		// Attempt 2: BtbN (Fallback)
		if !downloadSuccess {
			os.Remove(tempZip)
			logInfo("Falling back to download FFmpeg from BtbN (GitHub)...")
			
			btbnURL := "https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-win64-gpl.zip"
			logInfo("Fetching FFmpeg SHA256 checksum from BtbN...")
			expectedHash, err = fetchBtbNChecksum()
			if err == nil {
				if err := downloadFFmpeg(btbnURL, tempZip); err == nil {
					logInfo("Verifying download integrity...")
					if err := verifyFileHash(tempZip, expectedHash, true); err == nil {
						downloadSuccess = true
					} else {
						logError("BtbN checksum verification failed: %v", err)
					}
				} else {
					logError("Failed to download FFmpeg from BtbN: %v", err)
				}
			} else {
				logError("Failed to fetch BtbN checksum: %v", err)
			}
		}
		
		if !downloadSuccess {
			os.Remove(tempZip)
			return fmt.Errorf("failed to download and verify FFmpeg from both Gyan.dev and BtbN")
		}

		logInfo("Extracting FFmpeg...")
		if err := extractFFmpegWin(tempZip, localPath); err != nil {
			os.Remove(tempZip)
			logError("Failed to extract FFmpeg: %v", err)
			return fmt.Errorf("failed to extract FFmpeg: %w", err)
		}
		os.Remove(tempZip)
	} else if runtime.GOOS == "linux" || runtime.GOOS == "android" {
		url := ffmpegLinuxURL
		if runtime.GOARCH == "arm64" {
			url = "https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-arm64-static.tar.xz"
		} else if runtime.GOARCH == "arm" {
			url = "https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-armel-static.tar.xz"
		}

		checksumURL := url + ".md5"
		logInfo("Fetching FFmpeg MD5 checksum...")
		expectedHash, err := fetchRemoteChecksum(checksumURL)
		if err != nil {
			logError("Failed to fetch FFmpeg checksum: %v", err)
			return fmt.Errorf("failed to fetch FFmpeg checksum: %w", err)
		}

		tempTar := filepath.Join(binDir, "ffmpeg-temp.tar.xz")
		if err := downloadFFmpeg(url, tempTar); err != nil {
			os.Remove(tempTar)
			logError("Failed to download FFmpeg: %v", err)
			return fmt.Errorf("failed to download FFmpeg: %w", err)
		}

		logInfo("Verifying download integrity...")
		if err := verifyFileHash(tempTar, expectedHash, false); err != nil {
			os.Remove(tempTar)
			logError("FFmpeg verification failed: %v", err)
			return fmt.Errorf("FFmpeg verification failed: %w", err)
		}

		logInfo("Extracting FFmpeg...")
		if err := extractFFmpegLinux(tempTar, binDir, localPath); err != nil {
			os.Remove(tempTar)
			logError("Failed to extract FFmpeg: %v", err)
			return fmt.Errorf("failed to extract FFmpeg: %w", err)
		}
		os.Remove(tempTar)
	} else {
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	logInfo("FFmpeg installed successfully.")
	return nil
}

func downloadFFmpeg(urlStr, dest string) error {
	logInfo("Downloading FFmpeg from %s", urlStr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var lastProgressTime int64 = time.Now().UnixNano()
	go func() {
		for {
			time.Sleep(5 * time.Second)
			if ctx.Err() != nil {
				return
			}
			last := atomic.LoadInt64(&lastProgressTime)
			if time.Since(time.Unix(0, last)) > 30*time.Second {
				logError("FFmpeg download timed out (no progress for 30s)")
				cancel()
				return
			}
		}
	}()

	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	size := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 64*1024)
	lastPrint := time.Now()

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := f.Write(buf[:n]); wErr != nil {
				return wErr
			}
			downloaded += int64(n)
			atomic.StoreInt64(&lastProgressTime, time.Now().UnixNano())

			if time.Since(lastPrint) > 100*time.Millisecond || downloaded == size {
				lastPrint = time.Now()
				percent := float64(downloaded) / float64(size) * 100
				barLen := 20
				filled := int(percent / 100 * float64(barLen))
				bar := strings.Repeat("=", filled) + strings.Repeat(" ", barLen-filled)
				mbDownloaded := float64(downloaded) / 1024 / 1024
				mbTotal := float64(size) / 1024 / 1024
				fmt.Printf("\r  [INFO] Downloading: [%s] %.1f%% (%.1fMB / %.1fMB)", bar, percent, mbDownloaded, mbTotal)
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}
	fmt.Println() // Print newline after progress bar
	logInfo("FFmpeg download finished successfully")
	return nil
}

func extractFFmpegWin(zipPath, destPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	var targetFile *zip.File
	for _, f := range r.File {
		if strings.HasSuffix(f.Name, "bin/ffmpeg.exe") {
			targetFile = f
			break
		}
	}

	if targetFile == nil {
		return fmt.Errorf("ffmpeg.exe not found in zip archive")
	}

	rc, err := targetFile.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, rc)
	return err
}

func extractFFmpegLinux(tarPath, binDir, destPath string) error {
	cmd := exec.Command("tar", "-xf", tarPath, "-C", binDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tar command failed: %w", err)
	}

	files, err := os.ReadDir(binDir)
	if err != nil {
		return err
	}

	var extractedFolder string
	for _, f := range files {
		if f.IsDir() && strings.HasPrefix(f.Name(), "ffmpeg-") {
			extractedFolder = filepath.Join(binDir, f.Name())
			break
		}
	}

	if extractedFolder == "" {
		return fmt.Errorf("ffmpeg folder not found after extraction")
	}

	extractedFFmpeg := filepath.Join(extractedFolder, "ffmpeg")
	if err := os.Rename(extractedFFmpeg, destPath); err != nil {
		return fmt.Errorf("failed to move ffmpeg binary: %w", err)
	}

	_ = os.RemoveAll(extractedFolder)
	return nil
}
