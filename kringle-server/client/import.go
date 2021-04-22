package client

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/rwcarlsen/goexif/exif"

	"kringle-server/stingle"
)

// ImportFiles encrypts and imports files.
func (c *Client) ImportFiles(patterns []string, dir string) error {
	li, err := c.glob(dir)
	if err != nil {
		return err
	}
	if len(li) != 1 || !li[0].IsDir {
		return fmt.Errorf("%s is not a directory", dir)
	}
	dst := li[0]
	pk := c.SecretKey.PublicKey()
	if dst.AlbumID != "" {
		if pk, err = c.albumPK(dst.AlbumID); err != nil {
			return err
		}
	}

	var files []string
	for _, p := range patterns {
		m, err := filepath.Glob(p)
		if err != nil {
			return err
		}
		files = append(files, m...)
	}
	for _, file := range files {
		if err := c.importFile(file, dst, pk); err != nil {
			return err
		}
	}
	fmt.Printf("Successfully imported %d file(s)\n", len(files))
	return nil
}

func (c *Client) importFile(file string, dst ListItem, pk stingle.PublicKey) error {
	fi, err := os.Stat(file)
	if err != nil {
		return err
	}

	hdr1 := stingle.NewHeader()
	_, fn := filepath.Split(file)
	hdr1.Filename = []byte(fn)
	hdr1.DataSize = fi.Size()
	switch ext := strings.ToLower(filepath.Ext(file)); ext {
	case ".jpg", ".jpeg", ".png", ".gif":
		hdr1.FileType = stingle.FileTypePhoto
	case ".mp4", ".mov":
		hdr1.FileType = stingle.FileTypeVideo
	default:
		hdr1.FileType = stingle.FileTypeGeneral
	}
	if hdr1.FileType == stingle.FileTypeVideo {
		// TODO: Set VideoDuration
		//hdr1.VideoDuration
	}

	thumbnail, err := c.makeThumbnail(file)
	if err != nil {
		return err
	}
	hdr2 := stingle.NewHeader()
	hdr2.DataSize = int64(len(thumbnail))
	hdr2.FileType = stingle.FileTypePhoto

	encHdrs, err := stingle.EncryptBase64Headers([]stingle.Header{hdr1, hdr2}, pk)
	if err != nil {
		return err
	}
	sFile := stingle.File{
		File:         makeSPFilename(),
		Version:      "1",
		DateCreated:  json.Number(strconv.FormatInt(time.Now().UnixNano()/1000000, 10)),
		DateModified: json.Number(strconv.FormatInt(time.Now().UnixNano()/1000000, 10)),
		Headers:      encHdrs,
		AlbumID:      dst.AlbumID,
		LocalOnly:    true,
	}
	if x, err := c.importExif(file); err == nil {
		if t, err := x.DateTime(); err == nil {
			sFile.DateCreated = json.Number(strconv.FormatInt(t.UnixNano()/1000000, 10))
		}
	}
	in, err := os.Open(file)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := c.encryptFile(in, sFile.File, hdr1, pk, false); err != nil {
		return err
	}
	if err := c.encryptFile(bytes.NewBuffer(thumbnail), sFile.File, hdr2, pk, true); err != nil {
		return err
	}
	commit, fs, err := c.fileSetForUpdate(dst.FileSet)
	if err != nil {
		return err
	}
	fs.Files[sFile.File] = &sFile
	return commit(true, nil)
}

func makeSPFilename() string {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b) + ".sp"
}

func (c *Client) makeThumbnail(filename string) ([]byte, error) {
	img, err := imaging.Open(filename, imaging.AutoOrientation(true))
	if err != nil {
		// TODO: Create thumbnails for videos.
		img = image.NewGray(image.Rect(0, 0, 307, 409))
	}
	img = imaging.Fill(img, 307, 409, imaging.Center, imaging.Lanczos)

	var buf bytes.Buffer
	if err := imaging.Encode(&buf, img, imaging.PNG); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (c *Client) albumPK(albumID string) (pk stingle.PublicKey, err error) {
	var al AlbumList
	if _, err = c.storage.ReadDataFile(c.fileHash(albumList), &al); err != nil {
		return pk, err
	}
	album, ok := al.Albums[albumID]
	if !ok {
		return pk, os.ErrNotExist
	}
	b, err := base64.StdEncoding.DecodeString(album.PublicKey)
	if err != nil {
		return pk, err
	}
	return stingle.PublicKeyFromBytes(b), nil
}

func (c *Client) importExif(file string) (x *exif.Exif, err error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return exif.Decode(f)
}

func (c *Client) encryptFile(in io.Reader, file string, hdr stingle.Header, pk stingle.PublicKey, thumb bool) error {
	fn := c.blobPath(file, thumb)
	dir, _ := filepath.Split(fn)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s-tmp-%d", fn, time.Now().UnixNano())
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_SYNC, 0600)
	if err != nil {
		return err
	}
	if err := stingle.EncryptHeader(out, hdr, pk); err != nil {
		out.Close()
		return err
	}
	w := stingle.EncryptFile(out, hdr)
	if _, err := io.Copy(w, in); err != nil {
		w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, fn)
}