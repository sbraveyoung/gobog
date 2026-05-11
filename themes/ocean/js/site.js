/* gobog · minimal theme · site.js
 * No framework, no bundler — runs straight off the CDN.
 *
 * Responsibilities:
 *   - mobile nav toggle
 *   - light/dark theme toggle (persists in localStorage; auto-follow OS
 *     when no override is set)
 *   - 中/EN language toggle (sets html[lang], persists choice)
 *   - reading-progress bar on .post-body
 *   - floating TOC: build from h2/h3, show button when ≥ 3 headings,
 *     slide-in drawer with backdrop, dismiss on click outside / link
 *     click / Escape; current-section highlight via IntersectionObserver
 *   - code-block "copy" button
 *   - MathJax 3 conditional loader (matrix-friendly via ams package),
 *     fires only when the post body contains $...$ / $$...$$ / \\( / \\[
 */
(function () {
  // --- mobile nav ---
  var navBtn = document.querySelector(".nav-toggle");
  var nav = document.querySelector(".site-nav");
  if (navBtn && nav) {
    navBtn.addEventListener("click", function () {
      var open = nav.classList.toggle("is-open");
      navBtn.setAttribute("aria-expanded", open ? "true" : "false");
    });
  }

  // --- theme toggle ---
  var themeBtn = document.querySelector(".theme-toggle");
  if (themeBtn) {
    themeBtn.addEventListener("click", function () {
      var cur = document.documentElement.getAttribute("data-theme");
      // Derive from prefers-color-scheme when no explicit attribute is
      // set, so the first click flips against what the user is seeing.
      if (!cur) {
        cur = window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
      }
      var next = cur === "dark" ? "light" : "dark";
      document.documentElement.setAttribute("data-theme", next);
      localStorage.setItem("gobog-theme", next);
    });
  }

  // --- language toggle ---
  var langBtn = document.querySelector(".lang-toggle");
  if (langBtn) {
    langBtn.addEventListener("click", function () {
      var cur = document.documentElement.getAttribute("lang") || "zh-cn";
      var next = cur.toLowerCase().indexOf("zh") === 0 ? "en" : "zh-cn";
      document.documentElement.setAttribute("lang", next);
      localStorage.setItem("gobog-lang", next);
    });
  }

  // --- post-only enhancements below this line ---
  var body = document.querySelector(".post-body");
  if (!body) return;

  // --- reading progress ---
  var bar = document.querySelector(".reading-progress");
  if (bar) {
    var update = function () {
      var rect = body.getBoundingClientRect();
      var total = body.offsetHeight - window.innerHeight;
      var pct = total > 0 ? Math.max(0, Math.min(1, (-rect.top) / total)) : 0;
      bar.style.transform = "scaleX(" + pct + ")";
    };
    document.addEventListener("scroll", update, { passive: true });
    window.addEventListener("resize", update);
    update();
  }

  // --- floating TOC ---
  // Build only when the article has ≥ 3 headings; otherwise the navigation
  // would be more noise than value.
  var headings = body.querySelectorAll("h2, h3");
  var toggle = document.querySelector(".toc-toggle");
  var panel = document.getElementById("post-toc");
  var backdrop = document.querySelector(".toc-backdrop");

  if (toggle && panel && backdrop && headings.length >= 3) {
    var list = panel.querySelector(".post-toc-list");
    var anchors = [];
    headings.forEach(function (h, i) {
      if (!h.id) h.id = "h-" + i;
      var li = document.createElement("li");
      li.className = "toc-" + h.tagName.toLowerCase();
      var a = document.createElement("a");
      a.href = "#" + h.id;
      a.textContent = h.textContent;
      a.dataset.target = h.id;
      li.appendChild(a);
      list.appendChild(li);
      anchors.push(a);
    });

    toggle.hidden = false;

    var openDrawer = function () {
      panel.classList.add("is-open");
      backdrop.classList.add("is-open");
      toggle.setAttribute("aria-expanded", "true");
    };
    var closeDrawer = function () {
      panel.classList.remove("is-open");
      backdrop.classList.remove("is-open");
      toggle.setAttribute("aria-expanded", "false");
    };
    toggle.addEventListener("click", function () {
      if (panel.classList.contains("is-open")) closeDrawer(); else openDrawer();
    });
    var closeBtn = panel.querySelector(".toc-close");
    if (closeBtn) closeBtn.addEventListener("click", closeDrawer);
    backdrop.addEventListener("click", closeDrawer);
    // Auto-close after picking a section, otherwise the drawer hides
    // the scroll target on small screens.
    list.addEventListener("click", function (e) {
      if (e.target.tagName === "A") closeDrawer();
    });
    document.addEventListener("keydown", function (e) {
      if (e.key === "Escape" && panel.classList.contains("is-open")) closeDrawer();
    });

    // Current-section highlight. IntersectionObserver fires as headings
    // cross into the top quarter of the viewport; we track the most
    // recent intersecting one.
    if ("IntersectionObserver" in window) {
      var active = null;
      var io = new IntersectionObserver(function (entries) {
        entries.forEach(function (e) {
          if (e.isIntersecting) active = e.target.id;
        });
        if (!active) return;
        anchors.forEach(function (a) {
          a.classList.toggle("is-current", a.dataset.target === active);
        });
      }, { rootMargin: "0px 0px -75% 0px" });
      headings.forEach(function (h) { io.observe(h); });
    }
  }

  // --- code "copy" button ---
  body.querySelectorAll("pre > code").forEach(function (code) {
    var pre = code.parentElement;
    pre.style.position = "relative";
    var btn = document.createElement("button");
    btn.type = "button";
    btn.className = "copy-btn";
    btn.textContent = "copy";
    btn.style.cssText = "position:absolute;top:6px;right:8px;font-size:11px;padding:2px 8px;border-radius:4px;border:1px solid var(--border);background:var(--bg-card);color:var(--fg-soft);cursor:pointer;opacity:0;transition:opacity .15s;";
    pre.addEventListener("mouseenter", function () { btn.style.opacity = "1"; });
    pre.addEventListener("mouseleave", function () { btn.style.opacity = "0"; });
    btn.addEventListener("click", function () {
      if (!navigator.clipboard) return;
      navigator.clipboard.writeText(code.innerText).then(function () {
        btn.textContent = "copied";
        setTimeout(function () { btn.textContent = "copy"; }, 1200);
      });
    });
    pre.appendChild(btn);
  });

  // --- MathJax 3, conditional ---
  // Matrix support comes via the `ams` package, included in the default
  // tex-mml-chtml bundle. `skipHtmlTags` keeps shell scripts and code
  // blocks from being treated as math when they happen to contain `$`.
  var raw = body.textContent || "";
  var hasMath = /\$\$|\$[^\s$][^$]*\$|\\\(|\\\[|\\begin\{/.test(raw);
  if (hasMath) {
    window.MathJax = {
      tex: {
        inlineMath: [["$", "$"], ["\\(", "\\)"]],
        displayMath: [["$$", "$$"], ["\\[", "\\]"]],
        processEscapes: true,
        packages: { "[+]": ["ams", "noerrors", "noundefined"] }
      },
      options: {
        skipHtmlTags: ["script", "noscript", "style", "textarea", "pre", "code"]
      },
      svg: { fontCache: "global" }
    };
    var s = document.createElement("script");
    s.src = "https://cdn.jsdelivr.net/npm/mathjax@3/es5/tex-mml-chtml.js";
    s.async = true;
    s.id = "mathjax-script";
    document.head.appendChild(s);
  }
})();
