package server

import (
	"encoding/json"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	ttemplate "text/template"

	"github.com/sbraveyoung/gobog/src/config"
)

// templateFuncs is the func map every theme template gets. `add` is used by
// the press theme's `{{printf "%02d" (add $i 2)}}` numbering; keeping the map
// global means new themes can rely on it without server-side coordination.
var templateFuncs = map[string]interface{}{
	"add": func(a, b int) int { return a + b },
	"sub": func(a, b int) int { return a - b },
	"mul": func(a, b int) int { return a * b },
}

// parseHTMLTemplate loads a theme HTML template (html/template, used for
// index/group pages where Go must escape interpolations).
func parseHTMLTemplate(path string) (*template.Template, error) {
	name := filepath.Base(path)
	return template.New(name).Funcs(template.FuncMap(templateFuncs)).ParseFiles(path)
}

// parseTextTemplate loads a theme text template (text/template, used for
// post.html where the body is pre-rendered HTML and must not be re-escaped).
func parseTextTemplate(path string) (*ttemplate.Template, error) {
	name := filepath.Base(path)
	return ttemplate.New(name).Funcs(ttemplate.FuncMap(templateFuncs)).ParseFiles(path)
}

// themeCookie is the cookie name used to remember the visitor's theme choice
// across requests. Its value is the bare theme directory name (e.g. "letter"),
// resolved against the same parent directory as [blog].theme.
const themeCookie = "gobog_theme"

// availableThemes lists the theme directories sitting alongside the
// configured theme. A directory only counts as a theme if it contains an
// index.html — that's the minimum the server's renderer needs.
func availableThemes() []string {
	parent, current := filepath.Split(filepath.Clean(config.C.Blog.Theme))
	parent = filepath.Clean(parent)
	if parent == "" || parent == "." {
		return []string{current}
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		return []string{current}
	}
	out := make([]string, 0, len(entries))
	seenCurrent := false
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if _, err := os.Stat(filepath.Join(parent, name, "index.html")); err != nil {
			continue
		}
		if name == current {
			seenCurrent = true
		}
		out = append(out, name)
	}
	if !seenCurrent {
		out = append(out, current)
	}
	sort.Strings(out)
	return out
}

// themeFor returns the filesystem path of the theme to use for this request.
// A valid gobog_theme cookie wins; otherwise the configured theme is used.
// The cookie value is rejected if it contains a path separator or doesn't
// resolve to a real theme directory next to the default one.
func themeFor(r *http.Request) string {
	if r == nil {
		return config.C.Blog.Theme
	}
	c, err := r.Cookie(themeCookie)
	if err != nil || c.Value == "" {
		return config.C.Blog.Theme
	}
	name := c.Value
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return config.C.Blog.Theme
	}
	parent, _ := filepath.Split(filepath.Clean(config.C.Blog.Theme))
	parent = filepath.Clean(parent)
	if parent == "" || parent == "." {
		return config.C.Blog.Theme
	}
	candidate := filepath.Join(parent, name)
	if _, err := os.Stat(filepath.Join(candidate, "index.html")); err != nil {
		return config.C.Blog.Theme
	}
	return candidate
}

func themesJSONHandler(w http.ResponseWriter, r *http.Request) {
	current := ""
	if c, err := r.Cookie(themeCookie); err == nil {
		current = c.Value
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"themes":  availableThemes(),
		"current": current,
		"default": filepath.Base(filepath.Clean(config.C.Blog.Theme)),
	})
}

func themeSwitcherJSHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write([]byte(themeSwitcherJS))
}

const themeSwitcherJS = `(function () {
  if (window.__gobogThemeSwitcherLoaded) return;
  window.__gobogThemeSwitcherLoaded = true;

  var COOKIE = 'gobog_theme';

  function setCookie(v) {
    var d = new Date();
    d.setFullYear(d.getFullYear() + 1);
    document.cookie = COOKIE + '=' + encodeURIComponent(v) +
      '; path=/; expires=' + d.toUTCString() + '; SameSite=Lax';
  }
  function clearCookie() {
    document.cookie = COOKIE + '=; path=/; expires=Thu, 01 Jan 1970 00:00:00 GMT; SameSite=Lax';
  }

  function injectStyle() {
    if (document.getElementById('gobog-theme-switcher-style')) return;
    var s = document.createElement('style');
    s.id = 'gobog-theme-switcher-style';
    s.textContent = [
      '.gobog-ts{position:fixed;right:16px;bottom:16px;z-index:99999;',
      'font:14px/1.2 -apple-system,BlinkMacSystemFont,"Helvetica Neue",Arial,"PingFang SC","Hiragino Sans GB","Microsoft YaHei",sans-serif;}',
      '.gobog-ts__btn{appearance:none;border:1px solid rgba(0,0,0,.15);background:#fff;color:#111;',
      'border-radius:999px;padding:8px 14px;cursor:pointer;box-shadow:0 2px 10px rgba(0,0,0,.12);}',
      '.gobog-ts__btn:hover{background:#f5f5f5;}',
      '.gobog-ts__panel{position:absolute;right:0;bottom:46px;min-width:160px;background:#fff;color:#111;',
      'border:1px solid rgba(0,0,0,.12);border-radius:10px;box-shadow:0 10px 30px rgba(0,0,0,.15);',
      'padding:6px;display:none;max-height:60vh;overflow:auto;}',
      '.gobog-ts--open .gobog-ts__panel{display:block;}',
      '.gobog-ts__item{display:flex;align-items:center;justify-content:space-between;width:100%;',
      'text-align:left;padding:8px 12px;background:transparent;border:0;border-radius:6px;',
      'cursor:pointer;color:inherit;font:inherit;text-transform:capitalize;}',
      '.gobog-ts__item:hover{background:rgba(0,0,0,.06);}',
      '.gobog-ts__item--active{font-weight:600;}',
      '.gobog-ts__check{opacity:.6;margin-left:12px;}',
      '@media (prefers-color-scheme: dark){',
      '.gobog-ts__btn{background:#1c1c1c;color:#eee;border-color:rgba(255,255,255,.18);box-shadow:0 2px 10px rgba(0,0,0,.5);}',
      '.gobog-ts__btn:hover{background:#2a2a2a;}',
      '.gobog-ts__panel{background:#1c1c1c;color:#eee;border-color:rgba(255,255,255,.14);box-shadow:0 10px 30px rgba(0,0,0,.6);}',
      '.gobog-ts__item:hover{background:rgba(255,255,255,.08);}',
      '}'
    ].join('');
    document.head.appendChild(s);
  }

  function render(data) {
    if (!data || !Array.isArray(data.themes) || data.themes.length < 2) return;
    injectStyle();

    var def = data['default'] || '';
    var isStatic = !!data['static'];

    // In static mode we live inside /__themes/<name>/... — derive the
    // active theme from the URL prefix so the active state matches what
    // the user actually sees, not what the manifest happens to claim.
    var staticMatch = location.pathname.match(/^\/__themes\/([^\/]+)/);
    var current;
    if (isStatic) {
      current = staticMatch ? decodeURIComponent(staticMatch[1]) : def;
    } else {
      current = data.current || def;
    }

    var wrap = document.createElement('div');
    wrap.className = 'gobog-ts';

    var btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'gobog-ts__btn';
    btn.setAttribute('aria-haspopup', 'true');
    btn.setAttribute('aria-expanded', 'false');
    btn.textContent = '\u{1F3A8} 主题';
    wrap.appendChild(btn);

    var panel = document.createElement('div');
    panel.className = 'gobog-ts__panel';
    panel.setAttribute('role', 'menu');

    data.themes.forEach(function (name) {
      var item = document.createElement('button');
      item.type = 'button';
      item.className = 'gobog-ts__item' + (name === current ? ' gobog-ts__item--active' : '');
      item.setAttribute('role', 'menuitem');

      var label = document.createElement('span');
      label.textContent = name;
      item.appendChild(label);

      if (name === current) {
        var ck = document.createElement('span');
        ck.className = 'gobog-ts__check';
        ck.textContent = '✓';
        item.appendChild(ck);
      }

      item.addEventListener('click', function () {
        if (isStatic) {
          // Static export: no server to read a cookie, so navigate by URL.
          var path = location.pathname;
          var m = path.match(/^\/__themes\/[^\/]+(\/.*)?$/);
          var logical = m ? (m[1] || '/') : path;
          if (logical === '') logical = '/';
          var target;
          if (name === def) {
            target = logical;
          } else {
            target = '/__themes/' + encodeURIComponent(name) + logical;
          }
          location.href = target + location.search + location.hash;
          return;
        }
        if (name === def) {
          clearCookie();
        } else {
          setCookie(name);
        }
        location.reload();
      });
      panel.appendChild(item);
    });

    wrap.appendChild(panel);
    document.body.appendChild(wrap);

    function close() {
      wrap.classList.remove('gobog-ts--open');
      btn.setAttribute('aria-expanded', 'false');
    }
    btn.addEventListener('click', function (e) {
      e.stopPropagation();
      var open = wrap.classList.toggle('gobog-ts--open');
      btn.setAttribute('aria-expanded', open ? 'true' : 'false');
    });
    document.addEventListener('click', function (e) {
      if (!wrap.contains(e.target)) close();
    });
    document.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') close();
    });
  }

  function init() {
    try {
      fetch('/__themes.json', { credentials: 'same-origin' })
        .then(function (r) { return r.ok ? r.json() : null; })
        .then(render)
        .catch(function () {});
    } catch (e) { /* no fetch in ancient browsers — skip silently */ }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
`
