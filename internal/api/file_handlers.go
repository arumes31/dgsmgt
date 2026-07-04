package api

import (
	"dgsmgt/internal/auth"
	"dgsmgt/internal/models"
	"dgsmgt/internal/utils"
	"fmt"
	"net/http"
	"path"

	"github.com/gorilla/mux"
)

// File manager endpoints. All of them require the CanAccessFiles permission
// (admins always have it), so users only ever see files of servers they were
// granted file access on.

const maxEditableFileSize = 5 * 1024 * 1024 // 5 MB in-browser editor cap

func (a *API) fileAccess(w http.ResponseWriter, r *http.Request) (*models.Server, *auth.Claims, bool) {
	id := mux.Vars(r)["id"]
	claims := claimsFrom(r)
	perms, server, status := a.getAccess(claims, id)
	if status != http.StatusOK {
		writeAccessStatus(w, status)
		return nil, nil, false
	}
	if !perms.CanAccessFiles {
		utils.Forbidden(w, "Permission denied: no file access")
		return nil, nil, false
	}
	return server, claims, true
}

// FileListHandler lists a directory. Query: path (default /).
func (a *API) FileListHandler(w http.ResponseWriter, r *http.Request) {
	server, _, ok := a.fileAccess(w, r)
	if !ok {
		return
	}
	dir := r.URL.Query().Get("path")
	svc, ok := a.svcFor(w, server)
	if !ok {
		return
	}
	entries, err := svc.ListDir(r.Context(), server.ContainerID, dir)
	if err != nil {
		utils.BadRequest(w, "Cannot list directory: "+err.Error())
		return
	}
	utils.Success(w, map[string]interface{}{"path": dir, "entries": entries})
}

// FileReadHandler returns file content for the editor. Query: path.
func (a *API) FileReadHandler(w http.ResponseWriter, r *http.Request) {
	server, _, ok := a.fileAccess(w, r)
	if !ok {
		return
	}
	p := r.URL.Query().Get("path")
	svc, ok := a.svcFor(w, server)
	if !ok {
		return
	}
	data, err := svc.ReadFile(r.Context(), server.ContainerID, p, maxEditableFileSize)
	if err != nil {
		utils.BadRequest(w, "Cannot read file: "+err.Error())
		return
	}
	utils.Success(w, map[string]string{"path": p, "content": string(data)})
}

// FileWriteHandler saves editor content.
func (a *API) FileWriteHandler(w http.ResponseWriter, r *http.Request) {
	server, claims, ok := a.fileAccess(w, r)
	if !ok {
		return
	}
	var input struct {
		Path    string `json:"path" validate:"required"`
		Content string `json:"content"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	svc, ok := a.svcFor(w, server)
	if !ok {
		return
	}
	if err := svc.WriteFile(r.Context(), server.ContainerID, input.Path, []byte(input.Content)); err != nil {
		utils.BadRequest(w, "Cannot write file: "+err.Error())
		return
	}
	a.audit(r, claims, "file_write", auditOpts{Server: server, Details: input.Path, Success: true})
	utils.Success(w, map[string]string{"status": "ok"})
}

// FileDownloadHandler streams a file or directory as a tar archive.
func (a *API) FileDownloadHandler(w http.ResponseWriter, r *http.Request) {
	server, claims, ok := a.fileAccess(w, r)
	if !ok {
		return
	}
	p := r.URL.Query().Get("path")
	svc, ok := a.svcFor(w, server)
	if !ok {
		return
	}
	rc, name, err := svc.DownloadTar(r.Context(), server.ContainerID, p)
	if err != nil {
		utils.BadRequest(w, "Cannot download: "+err.Error())
		return
	}
	defer func() { _ = rc.Close() }()
	a.audit(r, claims, "file_download", auditOpts{Server: server, Details: p, Success: true})
	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.tar"`, path.Base(name)))
	buf := make([]byte, 32*1024)
	for {
		n, err := rc.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// FileUploadHandler accepts multipart uploads into a directory.
// Fields: path (dest dir), files (one or more).
func (a *API) FileUploadHandler(w http.ResponseWriter, r *http.Request) {
	server, claims, ok := a.fileAccess(w, r)
	if !ok {
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil { // 64MB in-memory cap; rest spills to disk
		utils.BadRequest(w, "Invalid upload")
		return
	}
	dest := r.FormValue("path")
	if dest == "" {
		dest = "/"
	}
	svc, ok := a.svcFor(w, server)
	if !ok {
		return
	}
	uploaded := []string{}
	if r.MultipartForm != nil {
		for _, headers := range r.MultipartForm.File {
			for _, hdr := range headers {
				f, err := hdr.Open()
				if err != nil {
					continue
				}
				err = svc.UploadFile(r.Context(), server.ContainerID, dest, hdr.Filename, f, hdr.Size)
				_ = f.Close()
				if err != nil {
					utils.BadRequest(w, "Upload failed for "+hdr.Filename+": "+err.Error())
					return
				}
				uploaded = append(uploaded, hdr.Filename)
			}
		}
	}
	a.audit(r, claims, "file_upload", auditOpts{Server: server,
		Details: fmt.Sprintf("Uploaded %d file(s) to %s", len(uploaded), dest), Success: true})
	utils.Success(w, map[string]interface{}{"uploaded": uploaded})
}

// FileExtractHandler uploads a tar archive and extracts it into a directory
// (unarchive). Fields: path, archive (tar file).
func (a *API) FileExtractHandler(w http.ResponseWriter, r *http.Request) {
	server, claims, ok := a.fileAccess(w, r)
	if !ok {
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		utils.BadRequest(w, "Invalid upload")
		return
	}
	dest := r.FormValue("path")
	if dest == "" {
		dest = "/"
	}
	file, _, err := r.FormFile("archive")
	if err != nil {
		utils.BadRequest(w, "archive field required (tar format)")
		return
	}
	defer func() { _ = file.Close() }()

	svc, ok := a.svcFor(w, server)
	if !ok {
		return
	}
	if err := svc.UploadTar(r.Context(), server.ContainerID, dest, file); err != nil {
		utils.BadRequest(w, "Extract failed: "+err.Error())
		return
	}
	a.audit(r, claims, "file_extract", auditOpts{Server: server, Details: "Extracted archive to " + dest, Success: true})
	utils.Success(w, map[string]string{"status": "ok"})
}

func (a *API) FileDeleteHandler(w http.ResponseWriter, r *http.Request) {
	server, claims, ok := a.fileAccess(w, r)
	if !ok {
		return
	}
	var input struct {
		Path string `json:"path" validate:"required"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	svc, ok := a.svcFor(w, server)
	if !ok {
		return
	}
	if err := svc.DeletePath(r.Context(), server.ContainerID, input.Path); err != nil {
		utils.BadRequest(w, "Delete failed: "+err.Error())
		return
	}
	a.audit(r, claims, "file_delete", auditOpts{Server: server, Details: input.Path, Success: true})
	utils.Success(w, map[string]string{"status": "ok"})
}

func (a *API) FileMkdirHandler(w http.ResponseWriter, r *http.Request) {
	server, claims, ok := a.fileAccess(w, r)
	if !ok {
		return
	}
	var input struct {
		Path string `json:"path" validate:"required"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	svc, ok := a.svcFor(w, server)
	if !ok {
		return
	}
	if err := svc.Mkdir(r.Context(), server.ContainerID, input.Path); err != nil {
		utils.BadRequest(w, "mkdir failed: "+err.Error())
		return
	}
	a.audit(r, claims, "file_mkdir", auditOpts{Server: server, Details: input.Path, Success: true})
	utils.Success(w, map[string]string{"status": "ok"})
}

func (a *API) FileMoveHandler(w http.ResponseWriter, r *http.Request) {
	server, claims, ok := a.fileAccess(w, r)
	if !ok {
		return
	}
	var input struct {
		From string `json:"from" validate:"required"`
		To   string `json:"to" validate:"required"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	svc, ok := a.svcFor(w, server)
	if !ok {
		return
	}
	if err := svc.MovePath(r.Context(), server.ContainerID, input.From, input.To); err != nil {
		utils.BadRequest(w, "Move failed: "+err.Error())
		return
	}
	a.audit(r, claims, "file_move", auditOpts{Server: server,
		Details: input.From + " -> " + input.To, Success: true})
	utils.Success(w, map[string]string{"status": "ok"})
}
