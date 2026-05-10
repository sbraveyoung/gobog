/* gobog · minimal theme · site.js
 * No framework, no bundler — runs straight from the CDN.
 *
 * Responsibilities:
 *   - mobile nav toggle
 *   - light/dark theme toggle (persists in localStorage; auto-follow OS
 *     when no override is set)
 *   - 中/EN language toggle (sets html[lang], persists choice)
 *   - reading-progress bar on .post-body
 *   - auto-built TOC for posts with 3+ headings
 *   - code-block "copy" button
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
      // If no explicit attribute, derive from prefers-color-scheme so the
      // first click flips against what the user is actually seeing.
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

  // --- reading progress (post pages only) ---
  var bar = document.querySelector(".reading-progress");
  var body = document.querySelector(".post-body");
  if (bar && body) {
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

  // --- auto TOC ---
  var toc = document.querySelector(".post-toc");
  if (toc && body) {
    var headings = body.querySelectorAll("h2, h3");
    if (headings.length >= 3) {
      var ol = toc.querySelector("ol");
      headings.forEach(function (h, i) {
        if (!h.id) h.id = "h-" + i;
        var li = document.createElement("li");
        li.className = "toc-" + h.tagName.toLowerCase();
        var a = document.createElement("a");
        a.href = "#" + h.id;
        a.textContent = h.textContent;
        li.appendChild(a);
        ol.appendChild(li);
      });
      toc.hidden = false;
    }
  }

  // --- code "copy" button ---
  if (body) {
    body.querySelectorAll("pre > code").forEach(function (code) {
      var pre = code.parentElement;
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
  }
})();
