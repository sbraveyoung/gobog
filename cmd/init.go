package cmd

import "os"

func InitWorkSpace(path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	if err := os.Chdir(path); err != nil {
		return err
	}
	for _, subDir := range []string{"posts", "images", "videos", "audios"} {
		if err := os.Mkdir(subDir, os.ModeDir); err != nil {
			return err
		}
	}
	return nil
}
