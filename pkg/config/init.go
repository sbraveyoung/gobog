package config

import "os"

var (
	DIRS = map[string]string{
		"drafts": "drafts",
		"posts":  "posts",
		"images": "images",
		"videos": "videos",
		"audios": "audios",
	}
)

func initWorkSpace(path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	if err := os.Chdir(path); err != nil {
		return err
	}
	for _, subDir := range DIRS {
		if err := os.Mkdir(subDir, os.ModeDir); err != nil {
			return err
		}
	}
	return nil
}
