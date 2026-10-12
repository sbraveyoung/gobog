package blog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sbraveyoung/gobog/src/config"
)

func TestFolderHiddenIsInheritedAndPreviewCanIncludeIt(t *testing.T) {
	root := t.TempDir()
	writeNote(t, filepath.Join(root, "post/public.md"), "---\ntitle: Public\nurl: /post/public\n---\nPublic\n")
	writeNote(t, filepath.Join(root, "post/secret/_folder.md"), "---\nhidden: true\n---\nControl\n")
	writeNote(t, filepath.Join(root, "post/secret/child/_folder.md"), "---\nhidden: false\n---\n")
	writeNote(t, filepath.Join(root, "post/secret/child/note.md"), "---\ntitle: Secret\nurl: /post/secret-note\ntags: [secret-tag]\n---\nSecret body\n")
	prev := config.C.Blog
	config.C.Blog.Source = root
	config.C.Blog.IncludeDrafts, config.C.Blog.IncludeHidden = false, false
	t.Cleanup(func() { config.C.Blog = prev })
	if err := Blog.Reload(); err != nil {
		t.Fatal(err)
	}
	if Blog.FindByURL("/post/secret-note") != nil || Blog.Wiki().ResolveNote("Secret") != "" || len(Blog.PostsByTag("secret-tag")) != 0 {
		t.Fatal("hidden folder leaked through route/wiki/tags")
	}
	if len(Blog.AllPosts()) != 1 {
		t.Fatal("control note was published or public note lost")
	}
	config.C.Blog.IncludeHidden = true
	if err := Blog.Reload(); err != nil {
		t.Fatal(err)
	}
	if Blog.FindByURL("/post/secret-note") == nil || len(Blog.AllPosts()) != 2 {
		t.Fatal("preview failed to include hidden note, or included controls")
	}
	config.C.Blog.IncludeHidden = false
	if err := os.Remove(filepath.Join(root, "post/secret/_folder.md")); err != nil {
		t.Fatal(err)
	}
	if err := Blog.Reload(); err != nil {
		t.Fatal(err)
	}
	if Blog.FindByURL("/post/secret-note") == nil {
		t.Fatal("removing folder veto did not restore article")
	}
}

func TestRootPolicyHidesBothPostsAndPages(t *testing.T) {
	root := t.TempDir()
	writeNote(t, filepath.Join(root, "post/note.md"), "---\nurl: /post/note\n---\nbody")
	writeNote(t, filepath.Join(root, "pages/about.md"), "---\nurl: /about\n---\nbody")
	writeNote(t, filepath.Join(root, "_folder.md"), "---\nhidden: true\n---\n")
	prev := config.C.Blog
	config.C.Blog.Source, config.C.Blog.IncludeHidden = root, false
	t.Cleanup(func() { config.C.Blog = prev })
	if err := Blog.Reload(); err != nil {
		t.Fatal(err)
	}
	if len(Blog.AllPosts()) != 0 || len(Blog.Articles(BlogTypes["page"])) != 0 {
		t.Fatal("root folder veto leaked notes")
	}
}

func TestInvalidMetadataFailsBuildInsteadOfSilentlyDroppingContent(t *testing.T) {
	for _, meta := range []string{"", "hiden: true", "hidden: null", "hidden: perhaps", "hidden: [true]", "hidden: [", "hidden: true\nhidden: false"} {
		t.Run(meta, func(t *testing.T) {
			root := t.TempDir()
			writeNote(t, filepath.Join(root, "post/bad/_folder.md"), "---\n"+meta+"\n---\n")
			prev := config.C.Blog
			config.C.Blog.Source, config.C.Blog.IncludeHidden = root, false
			t.Cleanup(func() { config.C.Blog = prev })
			if err := Blog.Reload(); err == nil {
				t.Fatal("invalid folder visibility accepted")
			}
		})
	}
}

func TestPagesHonorAllPublicationSwitches(t *testing.T) {
	for _, meta := range []string{"draft: true", "publish: false", "hidden: true", "private: true"} {
		t.Run(meta, func(t *testing.T) {
			root := t.TempDir()
			writeNote(t, filepath.Join(root, "post/public.md"), "---\nurl: /post/public\n---\nbody")
			writeNote(t, filepath.Join(root, "pages/secret.md"), "---\nurl: /secret\n"+meta+"\n---\nbody")
			prev, export := config.C.Blog, config.ExportDir
			config.C.Blog.Source, config.C.Blog.IncludeHidden, config.C.Blog.IncludeDrafts = root, false, false
			config.ExportDir = "export"
			t.Cleanup(func() { config.C.Blog = prev; config.ExportDir = export })
			if err := Blog.Reload(); err != nil {
				t.Fatal(err)
			}
			if Blog.FindByURL("/secret") != nil || Blog.Wiki().ResolveNote("secret") != "" {
				t.Fatal("unpublished page leaked")
			}
		})
	}
}
