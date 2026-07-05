// Package backup creates, restores and prunes volume backups for managed
// servers. A backup is a single .tar.gz containing every mount of the
// container plus a manifest, so it can be restored onto a recreated container.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"dgsmgt/internal/config"
	"dgsmgt/internal/docker"
	"dgsmgt/internal/models"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/pkg/sftp"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"gorm.io/gorm"
)

type Manifest struct {
	ServerName string    `json:"server_name"`
	CreatedAt  time.Time `json:"created_at"`
	Mounts     []string  `json:"mounts"` // destination paths inside the container, by index
}

type Manager struct {
	db     *gorm.DB
	nodes  *docker.Manager
	cfg    *config.Config
	logger *zap.Logger
}

func NewManager(db *gorm.DB, nodes *docker.Manager, cfg *config.Config, logger *zap.Logger) *Manager {
	_ = os.MkdirAll(cfg.BackupPath, 0750)
	return &Manager{db: db, nodes: nodes, cfg: cfg, logger: logger}
}

func (m *Manager) localPath(fileName string) string {
	return filepath.Join(m.cfg.BackupPath, filepath.Base(fileName))
}

// Run creates a backup for a server and applies retention. Blocking; callers
// usually run it in a goroutine (the API layer records audit + notification).
func (m *Manager) Run(ctx context.Context, server *models.Server, note string) (*models.Backup, error) {
	rec := &models.Backup{
		ServerID:   server.ID,
		ServerName: server.Name,
		Status:     "running",
		Target:     server.BackupTarget,
		Note:       note,
	}
	if err := m.db.Create(rec).Error; err != nil {
		return nil, err
	}
	// Include the unique backup ID so two backups of the same server started in
	// the same second (e.g. scheduled + manual) can't collide on one file path.
	rec.FileName = fmt.Sprintf("%s-%s-%d.tar.gz", server.Name, time.Now().Format("20060102-150405"), rec.ID)

	err := m.create(ctx, server, rec)
	if err != nil {
		rec.Status = "failed"
		rec.Error = err.Error()
	} else {
		rec.Status = "done"
	}
	m.db.Save(rec)

	if err == nil {
		m.applyRetention(server)
	}
	return rec, err
}

func (m *Manager) create(ctx context.Context, server *models.Server, rec *models.Backup) error {
	svc, err := m.nodes.For(server.NodeID)
	if err != nil {
		return err
	}

	inspect, err := svc.Inspect(ctx, server.ContainerID)
	if err != nil {
		return fmt.Errorf("inspect: %w", err)
	}

	mounts := []string{}
	for _, mnt := range inspect.Mounts {
		mounts = append(mounts, mnt.Destination)
	}
	if len(mounts) == 0 {
		return fmt.Errorf("server has no volumes to back up")
	}

	outPath := m.localPath(rec.FileName)
	f, err := os.Create(outPath) // #nosec G304 -- path constrained to BackupPath
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	manifest, _ := json.Marshal(Manifest{
		ServerName: server.Name,
		CreatedAt:  time.Now(),
		Mounts:     mounts,
	})
	if err := tw.WriteHeader(&tar.Header{
		Name: "manifest.json", Mode: 0644, Size: int64(len(manifest)), ModTime: time.Now(),
	}); err != nil {
		return err
	}
	if _, err := tw.Write(manifest); err != nil {
		return err
	}

	for i, dest := range mounts {
		rc, _, err := svc.DownloadTar(ctx, server.ContainerID, dest)
		if err != nil {
			return fmt.Errorf("copying %s: %w", dest, err)
		}
		tr := tar.NewReader(rc)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				_ = rc.Close()
				return err
			}
			hdr.Name = fmt.Sprintf("volumes/%d/%s", i, hdr.Name)
			if err := tw.WriteHeader(hdr); err != nil {
				_ = rc.Close()
				return err
			}
			if _, err := io.Copy(tw, tr); err != nil { // #nosec G110 -- backups are size-bounded by volume size
				_ = rc.Close()
				return err
			}
		}
		_ = rc.Close()
	}

	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	if fi, err := os.Stat(outPath); err == nil {
		rec.SizeBytes = fi.Size()
	}

	switch server.BackupTarget {
	case "s3":
		return m.uploadS3(ctx, outPath, rec.FileName)
	case "sftp":
		return m.uploadSFTP(outPath, rec.FileName)
	}
	return nil
}

// Restore streams a backup back into the container's mounts.
func (m *Manager) Restore(ctx context.Context, server *models.Server, rec *models.Backup) error {
	local := m.localPath(rec.FileName)
	if _, err := os.Stat(local); err != nil {
		// Try fetching from remote target
		switch rec.Target {
		case "s3":
			if err := m.downloadS3(ctx, rec.FileName, local); err != nil {
				return fmt.Errorf("backup file not available locally or on S3: %w", err)
			}
		case "sftp":
			if err := m.downloadSFTP(rec.FileName, local); err != nil {
				return fmt.Errorf("backup file not available locally or on SFTP: %w", err)
			}
		default:
			return fmt.Errorf("backup file missing: %s", rec.FileName)
		}
	}

	manifest, err := m.readManifest(local)
	if err != nil {
		return err
	}

	svc, err := m.nodes.For(server.NodeID)
	if err != nil {
		return err
	}

	for i, dest := range manifest.Mounts {
		prefix := fmt.Sprintf("volumes/%d/", i)
		pr, pw := io.Pipe()

		go func() {
			err := m.streamSubTar(local, prefix, pw)
			_ = pw.CloseWithError(err)
		}()

		parent := path.Dir(dest)
		err := svc.UploadTar(ctx, server.ContainerID, parent, pr)
		// Close the read end so streamSubTar unblocks (no goroutine/file leak)
		// if UploadTar returned before draining the pipe.
		_ = pr.Close()
		if err != nil {
			return fmt.Errorf("restoring %s: %w", dest, err)
		}
	}
	return nil
}

func (m *Manager) readManifest(file string) (*Manifest, error) {
	f, err := os.Open(file) // #nosec G304
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err != nil {
			return nil, fmt.Errorf("manifest not found")
		}
		if hdr.Name == "manifest.json" {
			var man Manifest
			if err := json.NewDecoder(io.LimitReader(tr, 1<<20)).Decode(&man); err != nil {
				return nil, err
			}
			return &man, nil
		}
	}
}

// streamSubTar re-tars all entries under prefix (with the prefix stripped) to w.
func (m *Manager) streamSubTar(file, prefix string, w io.Writer) error {
	f, err := os.Open(file) // #nosec G304
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	tw := tar.NewWriter(w)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if !strings.HasPrefix(hdr.Name, prefix) {
			continue
		}
		hdr.Name = strings.TrimPrefix(hdr.Name, prefix)
		if hdr.Name == "" {
			continue
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := io.Copy(tw, tr); err != nil { // #nosec G110
			return err
		}
	}
	return tw.Close()
}

func (m *Manager) applyRetention(server *models.Server) {
	if server.BackupRetention <= 0 {
		return
	}
	var old []models.Backup
	m.db.Where("server_id = ? AND status = ?", server.ID, "done").
		Order("created_at desc").Offset(server.BackupRetention).Find(&old)
	// One shared remote connection for the whole batch: pruning N backups
	// must not perform N SSH handshakes.
	rc := &remoteClients{}
	defer rc.close()
	for _, b := range old {
		m.deleteBackup(&b, rc)
	}
}

// remoteClients lazily caches per-target connections so batch deletions
// reuse one S3 client / SSH session instead of dialing per file. A failed
// dial is remembered and not retried within the batch.
type remoteClients struct {
	s3        *minio.Client
	s3Tried   bool
	sftp      *sftp.Client
	sftpConn  *ssh.Client
	sftpTried bool
}

func (rc *remoteClients) close() {
	if rc.sftp != nil {
		_ = rc.sftp.Close()
	}
	if rc.sftpConn != nil {
		_ = rc.sftpConn.Close()
	}
}

// DeleteBackup removes the record, the local file and — for remote targets —
// the uploaded object, so retention pruning cleans S3/SFTP too.
func (m *Manager) DeleteBackup(b *models.Backup) {
	rc := &remoteClients{}
	defer rc.close()
	m.deleteBackup(b, rc)
}

func (m *Manager) deleteBackup(b *models.Backup, rc *remoteClients) {
	switch b.Target {
	case "s3":
		if !rc.s3Tried {
			rc.s3Tried = true
			if cli, err := m.s3Client(); err == nil {
				rc.s3 = cli
			}
		}
		if rc.s3 != nil {
			if err := rc.s3.RemoveObject(context.Background(), m.cfg.S3Bucket, b.FileName,
				minio.RemoveObjectOptions{}); err != nil {
				m.logger.Warn("failed to delete S3 backup object", zap.String("file", b.FileName), zap.Error(err))
			}
		}
	case "sftp":
		if !rc.sftpTried {
			rc.sftpTried = true
			if cli, conn, err := m.sftpClient(); err == nil {
				rc.sftp, rc.sftpConn = cli, conn
			}
		}
		if rc.sftp != nil {
			if err := rc.sftp.Remove(path.Join(m.cfg.SFTPPath, b.FileName)); err != nil {
				m.logger.Warn("failed to delete SFTP backup file", zap.String("file", b.FileName), zap.Error(err))
			}
		}
	}
	_ = os.Remove(m.localPath(b.FileName))
	m.db.Unscoped().Delete(b)
}

func (m *Manager) s3Client() (*minio.Client, error) {
	if m.cfg.S3Endpoint == "" {
		return nil, fmt.Errorf("S3 not configured (S3_ENDPOINT)")
	}
	return minio.New(m.cfg.S3Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(m.cfg.S3AccessKey, m.cfg.S3SecretKey, ""),
		Secure: m.cfg.S3UseSSL,
	})
}

func (m *Manager) uploadS3(ctx context.Context, localPath, name string) error {
	cli, err := m.s3Client()
	if err != nil {
		return err
	}
	_, err = cli.FPutObject(ctx, m.cfg.S3Bucket, name, localPath, minio.PutObjectOptions{
		ContentType: "application/gzip",
	})
	return err
}

func (m *Manager) downloadS3(ctx context.Context, name, localPath string) error {
	cli, err := m.s3Client()
	if err != nil {
		return err
	}
	return cli.FGetObject(ctx, m.cfg.S3Bucket, name, localPath, minio.GetObjectOptions{})
}

func (m *Manager) sftpClient() (*sftp.Client, *ssh.Client, error) {
	if m.cfg.SFTPHost == "" {
		return nil, nil, fmt.Errorf("SFTP not configured (SFTP_HOST)")
	}

	// Host key verification is mandatory: pin via SFTP_HOST_KEY
	// (authorized_keys format), or explicitly opt out.
	var hostKeyCallback ssh.HostKeyCallback
	switch {
	case m.cfg.SFTPHostKey != "":
		pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(m.cfg.SFTPHostKey))
		if err != nil {
			return nil, nil, fmt.Errorf("invalid SFTP_HOST_KEY (expected authorized_keys format): %w", err)
		}
		hostKeyCallback = ssh.FixedHostKey(pub)
	case m.cfg.SFTPInsecure:
		hostKeyCallback = ssh.InsecureIgnoreHostKey() // #nosec G106 -- explicit opt-in via SFTP_INSECURE_SKIP_VERIFY
	default:
		return nil, nil, fmt.Errorf("SFTP host key verification required: set SFTP_HOST_KEY (get it via ssh-keyscan) or SFTP_INSECURE_SKIP_VERIFY=true")
	}

	sshCfg := &ssh.ClientConfig{
		User:            m.cfg.SFTPUser,
		Auth:            []ssh.AuthMethod{ssh.Password(m.cfg.SFTPPassword)},
		HostKeyCallback: hostKeyCallback,
		Timeout:         10 * time.Second,
	}
	conn, err := ssh.Dial("tcp", m.cfg.SFTPHost+":"+m.cfg.SFTPPort, sshCfg)
	if err != nil {
		return nil, nil, err
	}
	cli, err := sftp.NewClient(conn)
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	return cli, conn, nil
}

func (m *Manager) uploadSFTP(localPath, name string) error {
	cli, conn, err := m.sftpClient()
	if err != nil {
		return err
	}
	defer func() { _ = cli.Close(); _ = conn.Close() }()

	_ = cli.MkdirAll(m.cfg.SFTPPath)
	dst, err := cli.Create(path.Join(m.cfg.SFTPPath, name))
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()
	src, err := os.Open(localPath) // #nosec G304
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	_, err = io.Copy(dst, src)
	return err
}

func (m *Manager) downloadSFTP(name, localPath string) error {
	cli, conn, err := m.sftpClient()
	if err != nil {
		return err
	}
	defer func() { _ = cli.Close(); _ = conn.Close() }()

	src, err := cli.Open(path.Join(m.cfg.SFTPPath, name))
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	dst, err := os.Create(localPath) // #nosec G304
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()
	_, err = io.Copy(dst, src)
	return err
}
