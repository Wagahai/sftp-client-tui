package sftp

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type ProgressFn func(sent, total int64)

// Upload copies a local file to the remote SFTP server.
func (c *Client) Upload(localPath, remotePath string, progress ProgressFn) error {
	src, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open local: %w", err)
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return err
	}
	total := info.Size()

	// Ensure remote parent directory exists.
	if err := c.SFTP.MkdirAll(filepath.Dir(remotePath)); err != nil {
		return fmt.Errorf("mkdir remote: %w", err)
	}

	dst, err := c.SFTP.Create(remotePath)
	if err != nil {
		return fmt.Errorf("create remote: %w", err)
	}
	defer dst.Close()

	buf := make([]byte, 32*1024)
	var sent int64
	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			_, writeErr := dst.Write(buf[:n])
			if writeErr != nil {
				return fmt.Errorf("write remote: %w", writeErr)
			}
			sent += int64(n)
			if progress != nil {
				progress(sent, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("read local: %w", readErr)
		}
	}
	return nil
}

// Download copies a remote file to the local filesystem.
func (c *Client) Download(remotePath, localPath string, progress ProgressFn) error {
	src, err := c.SFTP.Open(remotePath)
	if err != nil {
		return fmt.Errorf("open remote: %w", err)
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return err
	}
	total := info.Size()

	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("mkdir local: %w", err)
	}

	dst, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("create local: %w", err)
	}
	defer dst.Close()

	buf := make([]byte, 32*1024)
	var sent int64
	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			_, writeErr := dst.Write(buf[:n])
			if writeErr != nil {
				return fmt.Errorf("write local: %w", writeErr)
			}
			sent += int64(n)
			if progress != nil {
				progress(sent, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("read remote: %w", readErr)
		}
	}
	return nil
}
