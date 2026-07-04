package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
)

func containerTypesCopyOpts() container.CopyToContainerOptions {
	return container.CopyToContainerOptions{AllowOverwriteDirWithFile: true}
}

// File manager backed by the Docker API (CopyFrom/CopyTo) plus exec for
// listing and mutations, so it works for any volume without host path access.

type FileEntry struct {
	Name    string `json:"name"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime string `json:"mod_time"`
}

// CleanContainerPath normalizes and validates a path inside the container.
func CleanContainerPath(p string) (string, error) {
	if p == "" {
		p = "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	cleaned := path.Clean(p)
	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("invalid path")
	}
	return cleaned, nil
}

// ListDir lists a directory inside the container using `ls -lAn`.
func (s *Service) ListDir(ctx context.Context, containerID, dir string) ([]FileEntry, error) {
	dir, err := CleanContainerPath(dir)
	if err != nil {
		return nil, err
	}
	out, err := s.Exec(ctx, containerID, []string{"ls", "-lAn", dir})
	if err != nil {
		return nil, err
	}
	entries := []FileEntry{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "total") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}
		mode := fields[0]
		size, _ := strconv.ParseInt(fields[4], 10, 64)
		// name = everything after the 8th field (dates contain spaces)
		nameIdx := strings.Index(line, fields[8])
		name := line
		if nameIdx >= 0 {
			name = line[nameIdx:]
			parts := strings.SplitN(name, " ", 2)
			if len(parts) == 2 {
				name = parts[1]
			}
		}
		// symlinks: "name -> target"
		if i := strings.Index(name, " -> "); i > 0 {
			name = name[:i]
		}
		name = strings.TrimSpace(name)
		if name == "" || name == "." || name == ".." {
			continue
		}
		entries = append(entries, FileEntry{
			Name:    name,
			IsDir:   strings.HasPrefix(mode, "d"),
			Size:    size,
			Mode:    mode,
			ModTime: strings.Join(fields[5:8], " "),
		})
	}
	return entries, nil
}

// ReadFile reads a single file (capped at maxBytes) via the tar copy API.
func (s *Service) ReadFile(ctx context.Context, containerID, filePath string, maxBytes int64) ([]byte, error) {
	filePath, err := CleanContainerPath(filePath)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	rc, stat, err := s.cli.CopyFromContainer(ctx, containerID, filePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()

	if stat.Mode.IsDir() {
		return nil, fmt.Errorf("path is a directory")
	}
	if maxBytes > 0 && stat.Size > maxBytes {
		return nil, fmt.Errorf("file too large (%d bytes, limit %d)", stat.Size, maxBytes)
	}

	tr := tar.NewReader(rc)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag == tar.TypeReg {
			if maxBytes <= 0 { // 0 = unlimited
				return io.ReadAll(tr)
			}
			data, err := io.ReadAll(io.LimitReader(tr, maxBytes+1))
			if err != nil {
				return nil, err
			}
			if int64(len(data)) > maxBytes {
				return nil, fmt.Errorf("file too large")
			}
			return data, nil
		}
	}
	return nil, fmt.Errorf("file not found in archive")
}

// WriteFile writes content to a file inside the container via CopyToContainer.
func (s *Service) WriteFile(ctx context.Context, containerID, filePath string, content []byte) error {
	filePath, err := CleanContainerPath(filePath)
	if err != nil {
		return err
	}
	dir := path.Dir(filePath)
	name := path.Base(filePath)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{
		Name:    name,
		Mode:    0644,
		Size:    int64(len(content)),
		ModTime: time.Now(),
	}); err != nil {
		return err
	}
	if _, err := tw.Write(content); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return s.cli.CopyToContainer(ctx, containerID, dir, &buf, containerTypesCopyOpts())
}

// DownloadTar streams a path (file or directory) out of the container as a tar.
func (s *Service) DownloadTar(ctx context.Context, containerID, srcPath string) (io.ReadCloser, string, error) {
	srcPath, err := CleanContainerPath(srcPath)
	if err != nil {
		return nil, "", err
	}
	rc, stat, err := s.cli.CopyFromContainer(ctx, containerID, srcPath)
	if err != nil {
		return nil, "", err
	}
	return rc, stat.Name, nil
}

// UploadTar extracts an uploaded tar stream into destDir inside the container.
func (s *Service) UploadTar(ctx context.Context, containerID, destDir string, content io.Reader) error {
	destDir, err := CleanContainerPath(destDir)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	return s.cli.CopyToContainer(ctx, containerID, destDir, content, containerTypesCopyOpts())
}

// UploadFile places a single uploaded file into destDir.
func (s *Service) UploadFile(ctx context.Context, containerID, destDir, fileName string, content io.Reader, size int64) error {
	destDir, err := CleanContainerPath(destDir)
	if err != nil {
		return err
	}
	pr, pw := io.Pipe()
	go func() {
		tw := tar.NewWriter(pw)
		err := tw.WriteHeader(&tar.Header{
			Name:    path.Base(fileName),
			Mode:    0644,
			Size:    size,
			ModTime: time.Now(),
		})
		if err == nil {
			_, err = io.Copy(tw, content)
		}
		if cerr := tw.Close(); err == nil {
			err = cerr
		}
		_ = pw.CloseWithError(err)
	}()

	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	return s.cli.CopyToContainer(ctx, containerID, destDir, pr, containerTypesCopyOpts())
}

// DeletePath removes a file or directory inside the container.
func (s *Service) DeletePath(ctx context.Context, containerID, target string) error {
	target, err := CleanContainerPath(target)
	if err != nil {
		return err
	}
	if target == "/" {
		return fmt.Errorf("refusing to delete /")
	}
	_, err = s.Exec(ctx, containerID, []string{"rm", "-rf", target})
	return err
}

// Mkdir creates a directory inside the container.
func (s *Service) Mkdir(ctx context.Context, containerID, dir string) error {
	dir, err := CleanContainerPath(dir)
	if err != nil {
		return err
	}
	_, err = s.Exec(ctx, containerID, []string{"mkdir", "-p", dir})
	return err
}

// MovePath renames/moves a file inside the container.
func (s *Service) MovePath(ctx context.Context, containerID, from, to string) error {
	from, err := CleanContainerPath(from)
	if err != nil {
		return err
	}
	to, err = CleanContainerPath(to)
	if err != nil {
		return err
	}
	_, err = s.Exec(ctx, containerID, []string{"mv", from, to})
	return err
}

// DirSizeKB returns `du -sk` for a path inside the container (kilobytes).
func (s *Service) DirSizeKB(ctx context.Context, containerID, dir string) (int64, error) {
	dir, err := CleanContainerPath(dir)
	if err != nil {
		return 0, err
	}
	out, err := s.Exec(ctx, containerID, []string{"du", "-sk", dir})
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return 0, fmt.Errorf("unexpected du output")
	}
	return strconv.ParseInt(fields[0], 10, 64)
}
