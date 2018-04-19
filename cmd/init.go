package cmd

import "os"

func InitWorkSpace(path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	if err := os.Chdir(path); err != nil {
		return err
	}
	for _, subDir := range []string{"post", "image", "video", "audio"} {
		if err := os.Mkdir(subDir, os.ModeDir); err != nil {
			return err
		}
	}
	return nil
}
