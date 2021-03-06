package core

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"github.com/callumj/weave/tools"
	"io"
	"log"
	"os"
	"path"
	"strings"
)

type Item struct {
	Start  int64
	Length int64
	Name   string
}

type ArchiveInfo struct {
	Items []Item
	Path  string
}

type archiveProcessCallback func(string, string, *tar.Reader) bool

func CompressArchive(archivePath, outPath string) bool {
	dupe, err := os.Create(outPath)
	if err != nil {
		log.Printf("Unable to open %v for writing\r\n", outPath)
		return false
	}
	defer dupe.Close()
	gzipPntr := gzip.NewWriter(dupe)
	defer gzipPntr.Close()

	basePntr, err := os.Open(archivePath)
	if err != nil {
		log.Printf("Unable to open %v for reading\r\n", archivePath)
		return false
	}
	defer basePntr.Close()

	io.Copy(gzipPntr, basePntr)

	return true
}

func MergeIntoBaseArchive(baseArchive ArchiveInfo, basedir string, contents []FileInfo, file string, definitiveList *ContentsInfo) bool {
	// tar pntr for copy
	dupe, err := os.Create(file)
	if err != nil {
		log.Printf("Unable to open %v for reading\r\n", file)
		return false
	}
	defer dupe.Close()

	tw := tar.NewWriter(dupe)
	defer tw.Close()

	basePntr, err := os.Open(baseArchive.Path)
	if err != nil {
		log.Printf("Unable to open archive %v for appending\r\n", baseArchive.Path)
		return false
	}
	defer basePntr.Close()

	if definitiveList != nil {
		// recursively copy, excluding as needed
		existingTar := tar.NewReader(basePntr)

		for {
			hdr, err := existingTar.Next()
			if err == io.EOF {
				// end of tar archive
				break
			}

			checkName := strings.TrimPrefix(hdr.Name, "/")
			found := false

			for _, item := range definitiveList.Contents {
				if item.RelPath == checkName {
					found = true
				}
			}

			if !found {
				continue
			}

			if err != nil {
				log.Printf("Failed to read tar for duping \r\n")
				return false
			}

			err = tw.WriteHeader(hdr)

			if err != nil {
				log.Printf("Failed copy header\r\n")
				return false
			}

			if _, err := io.Copy(tw, existingTar); err != nil {
				log.Printf("Unable to write %s (%v)\r\n", hdr.Name, err)
				return false
			}
		}
	} else {
		written, err := io.Copy(dupe, basePntr)
		if written == 0 {
			log.Printf("Warning: Did not write anything from %v to %v\r\n", baseArchive.Path, file)
			return false
		}

		if err != nil {
			log.Printf("Copy failed: %v\r\n", err)
			return false
		}

		// bump to the end
		dupe.Seek(-2<<9, os.SEEK_END)
	}

	// insert
	for _, item := range contents {
		res := writeFileToArchive(dupe, tw, item.AbsPath, basedir)
		if res == nil {
			log.Printf("Unable to add %v to new archive\r\n", item.AbsPath)
			return false
		}
	}

	return true
}

func CreateBaseArchive(basedir string, contents []FileInfo, file string) *ArchiveInfo {
	tarPntr, err := os.Create(file)
	if err != nil {
		log.Printf("Unable to open base archive %v\r\n", file)
		return nil
	}
	defer tarPntr.Close()

	tw := tar.NewWriter(tarPntr)
	defer tw.Close()
	total := len(contents)

	a := ArchiveInfo{Path: file}

	for index, info := range contents {
		item := writeFileToArchive(tarPntr, tw, info.AbsPath, basedir)
		if item == nil {
			log.Printf("Failed to add %v to base archive.\r\n", info.AbsPath)
			return nil
		}
		fmt.Printf("\rArchiving %v / %v", index+1, total)
		a.Items = append(a.Items, *item)
	}
	fmt.Println()

	return &a
}

func writeFileToArchive(tarPntr *os.File, tw *tar.Writer, file string, basedir string) *Item {
	curPos, err := tarPntr.Seek(0, 1)
	if err != nil {
		log.Println("Unable to determine current position")
		return nil
	}
	stat, err := os.Stat(file)
	if err != nil {
		log.Printf("Unable to query file %v\r\n", file)
		return nil
	}

	hdr := &tar.Header{
		Name:    strings.Replace(file, basedir, "", 1),
		Size:    stat.Size(),
		Mode:    775,
		ModTime: stat.ModTime(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		log.Printf("Unable to write TAR header for %v\r\n", hdr.Name)
		return nil
	}

	filePntr, err := os.Open(file)
	if err != nil {
		log.Printf("Unable to open %v for reading\r\n", hdr.Name)
		return nil
	}
	defer filePntr.Close()

	// read in chunks for memory
	buf := make([]byte, 1024)
	for {
		// read a chunk
		n, err := filePntr.Read(buf)
		if err != nil && err != io.EOF {
			log.Printf("Unable to open %v for reading\r\n", hdr.Name)
			return nil
		}
		if n == 0 {
			break
		}

		// write a chunk
		if _, err := tw.Write(buf[:n]); err != nil {
			log.Printf("Unable to write chunk for %v\r\n", hdr.Name)
			return nil
		}
	}

	endPos, err := tarPntr.Seek(0, 1)
	if err != nil {
		log.Println("Unable to determine end position")
		return nil
	}

	return &Item{Start: curPos, Length: (endPos - curPos), Name: hdr.Name}
}

func ExtractArchive(file, directory string) bool {
	return iterateOnArchive(file, directory, func(originalName, outputPath string, tarPntr *tar.Reader) bool {
		log.Printf("Extracting: %s\n", outputPath)

		totalPath := path.Dir(outputPath)
		if !tools.PathExists(totalPath) {
			os.MkdirAll(totalPath, 0770)
		}

		writePntr, err := os.Create(outputPath)
		if err != nil {
			log.Printf("Failed open handler for %s (%v)\r\n", outputPath, err)
			return false
		}

		if _, err := io.Copy(writePntr, tarPntr); err != nil {
			writePntr.Close()
			log.Printf("Unable to write %s (%v)\r\n", outputPath, err)
			return false
		}
		writePntr.Close()

		return true
	})
}

func FetchFile(archive, name string) string {
	contents := ""

	iterateOnArchive(archive, "", func(originalName, outputPath string, tarPntr *tar.Reader) bool {
		if name == originalName {
			buf := bytes.NewBuffer(nil)
			_, err := io.Copy(buf, tarPntr)
			if err != nil {
				log.Printf("Unable to read in %v\r\n", name)
				return false
			}

			contents = string(buf.Bytes())
			return false
		}
		return true
	})

	return contents
}

func iterateOnArchive(file, directory string, callback archiveProcessCallback) bool {
	filePntr, err := os.Open(file)
	if err != nil {
		log.Printf("Unable to open %v for reading\r\n", file)
		return false
	}
	defer filePntr.Close()

	gzipPntr, err := gzip.NewReader(filePntr)
	defer gzipPntr.Close()

	tarPntr := tar.NewReader(gzipPntr)

	for {
		hdr, err := tarPntr.Next()
		if err == io.EOF {
			// end of tar archive
			break
		}
		if err != nil {
			log.Printf("Failed to process %v archive", file)
			return false
		}
		var outputPath string
		if strings.HasSuffix(directory, "/") || strings.HasPrefix(hdr.Name, "/") {
			outputPath = strings.Join([]string{directory, hdr.Name}, "")
		} else {
			outputPath = strings.Join([]string{directory, hdr.Name}, "/")
		}

		res := callback(hdr.Name, outputPath, tarPntr)

		if res == false {
			return false
		}
	}

	return true
}
