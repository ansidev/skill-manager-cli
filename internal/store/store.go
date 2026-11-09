// Package store manages the local filesystem representation of installed skills.
package store

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// StoreSkill copies sourcePath to destinationPath when their directory trees
// differ. It returns false when destinationPath already has identical content.
func StoreSkill(sourcePath, destinationPath string) (bool, error) {
	equal, err := directoriesEqual(sourcePath, destinationPath)
	if err != nil {
		return false, err
	}
	if equal {
		return false, nil
	}

	parent := filepath.Dir(destinationPath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return false, fmt.Errorf("create store parent: %w", err)
	}

	temporaryPath, err := os.MkdirTemp(parent, ".skill-")
	if err != nil {
		return false, fmt.Errorf("create temporary skill directory: %w", err)
	}
	defer os.RemoveAll(temporaryPath)

	if err := copyDirectory(sourcePath, temporaryPath); err != nil {
		return false, err
	}

	backupPath := temporaryPath + ".previous"
	if _, err := os.Lstat(destinationPath); err == nil {
		if err := os.Rename(destinationPath, backupPath); err != nil {
			return false, fmt.Errorf("stage existing stored skill: %w", err)
		}
		defer os.RemoveAll(backupPath)
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("inspect stored skill: %w", err)
	}

	if err := os.Rename(temporaryPath, destinationPath); err != nil {
		return false, fmt.Errorf("replace stored skill: %w", err)
	}
	return true, nil
}

func directoriesEqual(leftPath, rightPath string) (bool, error) {
	leftInfo, err := os.Stat(leftPath)
	if err != nil {
		return false, fmt.Errorf("inspect source skill: %w", err)
	}
	if !leftInfo.IsDir() {
		return false, fmt.Errorf("source skill %q is not a directory", leftPath)
	}
	if _, err := os.Stat(rightPath); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("inspect stored skill: %w", err)
	}

	equal := true
	err = filepath.WalkDir(leftPath, func(leftFilePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relativePath, err := filepath.Rel(leftPath, leftFilePath)
		if err != nil {
			return err
		}
		if relativePath == "." {
			return nil
		}
		rightFilePath := filepath.Join(rightPath, relativePath)
		rightEntry, err := os.Lstat(rightFilePath)
		if os.IsNotExist(err) {
			equal = false
			return filepath.SkipDir
		}
		if err != nil {
			return err
		}
		if entry.IsDir() != rightEntry.IsDir() || entry.Type() != rightEntry.Mode().Type() {
			equal = false
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		leftContents, err := os.ReadFile(leftFilePath)
		if err != nil {
			return err
		}
		rightContents, err := os.ReadFile(rightFilePath)
		if err != nil {
			return err
		}
		if !bytes.Equal(leftContents, rightContents) {
			equal = false
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("compare skill directories: %w", err)
	}
	if !equal {
		return false, nil
	}

	err = filepath.WalkDir(rightPath, func(rightFilePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relativePath, err := filepath.Rel(rightPath, rightFilePath)
		if err != nil {
			return err
		}
		if relativePath == "." {
			return nil
		}
		if _, err := os.Lstat(filepath.Join(leftPath, relativePath)); os.IsNotExist(err) {
			equal = false
			return filepath.SkipDir
		} else {
			return err
		}
	})
	if err != nil {
		return false, fmt.Errorf("compare stored skill entries: %w", err)
	}
	return equal, nil
}

func copyDirectory(sourcePath, destinationPath string) error {
	return filepath.WalkDir(sourcePath, func(sourceFilePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relativePath, err := filepath.Rel(sourcePath, sourceFilePath)
		if err != nil {
			return err
		}
		destinationFilePath := filepath.Join(destinationPath, relativePath)
		if entry.IsDir() {
			return os.MkdirAll(destinationFilePath, 0o755)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(sourceFilePath)
			if err != nil {
				return err
			}
			return os.Symlink(target, destinationFilePath)
		}
		return copyFile(sourceFilePath, destinationFilePath)
	})
}

func copyFile(sourcePath, destinationPath string) error {
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	info, err := sourceFile.Stat()
	if err != nil {
		return err
	}
	destinationFile, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(destinationFile, sourceFile)
	closeErr := destinationFile.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
