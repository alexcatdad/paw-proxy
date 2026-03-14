# Landing Page Redesign Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite docs/index.html as an editorial-split landing page that tells the port-conflict origin story for a Show HN launch.

**Architecture:** Single HTML file rewrite with Tailwind CSS utilities + CSS custom properties for theming. Two-column prose/terminal editorial layout, 6-beat narrative structure. Vanilla JS for interactivity.

**Tech Stack:** HTML, Tailwind CSS (compiled), vanilla JS, JetBrains Mono (Google Fonts)

**Spec:** `docs/superpowers/specs/2026-03-14-landing-page-redesign-design.md`

---

## Chunk 1: Config + Scaffolding

### Task 1: Update Tailwind config font family

**Files:**
- Modify: `docs/tailwind.config.js:7-9`

- [ ] **Step 1: Update sans font family**

In `docs/tailwind.config.js`, change the `fontFamily.sans` array from `['Nunito', 'system-ui', 'sans-serif']` to `['system-ui', '-apple-system', 'sans-serif']`. Leave `mono` unchanged.

- [ ] **Step 2: Commit**

```bash
git add docs/tailwind.config.js
git commit -m "chore: replace Nunito with system-ui font stack"
```

### Task 2: Write the new index.html — head and styles

**Files:**
- Modify: `docs/index.html` (full rewrite — start from scratch)

- [ ] **Step 1: Write `<head>` and `<style>` block**

Replace the entire `docs/index.html` with the new page. Start with `<!DOCTYPE html>` through the closing `</style>` tag. Include:

- All SEO meta tags with updated copy:
  - `<title>paw-proxy — Stop Fighting Localhost</title>`
  - `<meta name="description" content="Named HTTPS domains for every local dev server. No more port conflicts, no more localhost confusion. Just prefix with &quot;up&quot;.">`
  - OG tags with title `paw-proxy — Stop Fighting Localhost` and matching description
  - Twitter Card tags (same text as OG)
  - `theme-color` meta: `#0a0a0a` for dark, `#fafaf9` for light
- Keep unchanged: canonical URL, OG image URL/dimensions, favicon (paw emoji), llms.txt links, JSON-LD (update description to: `Named HTTPS domains for local development. Stop fighting port conflicts — get https://myapp.test working in seconds with zero configuration.`)
- Google Fonts: load only JetBrains Mono (remove Nunito from URL)
- `<link rel="stylesheet" href="styles.css">`
- `<style>` block with:
  - CSS custom properties for dark mode (`:root` — `--bg-base: #0a0a0a`, `--bg-surface: #141414`, `--bg-terminal: #111111`, `--text-primary: #fafafa`, `--text-secondary: #a3a3a3`, `--text-muted: #525252`, `--border: #262626`, `--accent: #f97316`)
  - CSS custom properties for light mode (`:root:not(.dark)` — `--bg-base: #fafaf9`, `--bg-surface: #f5f5f4`, `--bg-terminal: #1c1917`, `--text-primary: #1c1917`, `--text-secondary: #57534e`, `--text-muted: #a8a29e`, `--border: #e7e5e4`, `--accent: #ea580c`)
  - **Note:** This is an intentional inversion — `:root` sets dark-mode defaults (the flagship), and `:root:not(.dark)` overrides to light. When JS removes the `dark` class from `<html>`, the `:not(.dark)` selector activates and light-mode variables take effect.
  - Two `<meta name="theme-color">` tags with `media` attributes (same pattern as current page): `<meta name="theme-color" content="#0a0a0a" media="(prefers-color-scheme: dark)">` and `<meta name="theme-color" content="#fafaf9" media="(prefers-color-scheme: light)">`
  - `body` styles: `background: var(--bg-base); color: var(--text-primary);`
  - Terminal dot styles (12px circles, same as current)
  - Cursor blink animation (`@keyframes blink { 50% { opacity: 0; } }`)
  - Fade-in animation (same as current: `opacity: 0; transform: translateY(24px); transition: ...`)
  - Smooth theme transitions on `body`

- [ ] **Step 2: Verify the head renders correctly**

Open `docs/index.html` in a browser. At this point you should see a black page (dark mode) or stone-white page (light mode, if OS prefers light). No content yet — just confirm the background color and that no console errors appear.

- [ ] **Step 3: Commit**

```bash
git add docs/index.html
git commit -m "feat(landing): scaffold new page with head, meta tags, and theme CSS"
```

---

## Chunk 2: Hero + Pain Beats (Body Sections 1-3)

### Task 3: Write the body opening, theme toggle, and Beat 1 (Hero)

**Files:**
- Modify: `docs/index.html` (append to existing)

- [ ] **Step 1: Add body opening, theme toggle, and hero section**

After the closing `</head>`, add:

- `<html lang="en" class="scroll-smooth dark">` — `dark` class goes on `<html>` (required by Tailwind's `darkMode: 'class'`). JS will remove it for light mode.
- `<body class="font-sans min-h-screen antialiased">` (no `dark` class here — it lives on `<html>`)
- Theme toggle button: fixed top-right, minimal style (no clay). Sun/moon SVG icons (same SVGs as current page, but styled with `var(--text-secondary)` / `text-amber-400`).
- `<div class="max-w-4xl mx-auto px-6 py-20">` — main content wrapper
- **Beat 1 — Hero section:**
  - `<header>` with centered layout
  - `🐾` paw emoji + `paw-proxy` title (no bouncing animation, just static)
  - `<h1>` with headline: "Stop fighting localhost." — large, bold, white (`var(--text-primary)`)
  - `<p>` with subline: "Named HTTPS domains for every local dev server. Zero config." — `var(--text-secondary)`
  - Hero terminal block: inset dark block showing `❯ up npm run dev` / `→ https://myapp.test ✓`
  - Brew install CTA with copy button: `brew install alexcatdad/tap/paw-proxy`

The hero does NOT use the two-column editorial layout — it's centered, full-width. The editorial split starts at Beat 2.

- [ ] **Step 2: Verify hero renders**

Open in browser. Should show: paw emoji, title, headline, subline, terminal block, install command. Dark background. Theme toggle should be visible but non-functional (JS not added yet).

- [ ] **Step 3: Commit**

```bash
git add docs/index.html
git commit -m "feat(landing): add hero section with headline and terminal demo"
```

### Task 4: Write Beat 2 (Port Conflicts) and Beat 3 (Worktree Confusion)

**Files:**
- Modify: `docs/index.html` (append after hero)

- [ ] **Step 1: Add Beat 2 — Port Conflicts**

After the hero `</header>`, add a `<section class="mb-24 fade-in">` with:

- Two-column grid: `grid md:grid-cols-2 gap-8 items-start`
- Left column (prose): styled with `var(--text-secondary)`, explaining 3 Next.js projects fighting over port 3000, OAuth breaking on :3001. Use a small section label/pill at top: "The problem" or similar.
- Right column (terminal): dark inset block (`var(--bg-terminal)`) with terminal dots, containing the verbatim terminal content from the spec (project-a → :3000, project-b → ⚠ port in use → :3001, OAuth redirect_uri_mismatch).

- [ ] **Step 2: Add Beat 3 — Worktree Confusion**

Another `<section class="mb-24 fade-in">` with same two-column grid:

- Left: prose about worktrees — same project, two branches, testing wrong code for 10 minutes.
- Right: terminal block with the verbatim spec content (main branch → :3000, feature branch → :3001, "which branch is this again?").

- [ ] **Step 3: Verify beats 2-3 render**

Open in browser. Should show two editorial-split sections below the hero. On desktop: prose left, terminal right. Resize to mobile width: should stack vertically (prose on top, terminal below).

- [ ] **Step 4: Commit**

```bash
git add docs/index.html
git commit -m "feat(landing): add port conflict and worktree pain sections"
```

---

## Chunk 3: Insight + Solution + How It Works (Body Sections 4-6)

### Task 5: Write Beat 4 (Insight) and Beat 5 (Solution)

**Files:**
- Modify: `docs/index.html` (append after Beat 3)

- [ ] **Step 1: Add Beat 4 — The Insight**

`<section class="mb-24 fade-in">` with two-column grid:

- Left: prose with bold callout **"The problem isn't ports. It's identity."** Explain that localhost:3000 says nothing about what's running. Named domains solve it. HTTPS comes free.
- Right: terminal block contrasting `localhost:3000 → ???` lines with `myapp.test → myapp (main)` etc.

- [ ] **Step 2: Add Beat 5 — The Solution**

`<section class="mb-24 fade-in">` with two-column grid:

- Left: prose with bold **"Just prefix with `up`."** Explain paw-proxy in one paragraph. Then weave in features as a natural list within the prose:
  - Auto SSL — trusted certs on-the-fly
  - WebSocket/HMR — hot reload just works
  - Docker Compose — auto-discovers services
  - Live dashboard at `_paw.test`
  - Worktree-aware naming
- Right: terminal block showing `up npm run dev → myapp.test`, `up docker compose up → frontend.shop.test + api.shop.test`, worktree conflict resolution.

- [ ] **Step 3: Verify beats 4-5 render**

Check the insight callout is visually prominent. Check the solution section reads as narrative, not a feature grid.

- [ ] **Step 4: Commit**

```bash
git add docs/index.html
git commit -m "feat(landing): add insight and solution sections with woven features"
```

### Task 6: Write Beat 6 (How It Works + Install), Prior Art, and Footer

**Files:**
- Modify: `docs/index.html` (append after Beat 5)

- [ ] **Step 1: Add architecture diagram section**

`<section class="mb-24 fade-in">` with centered layout (not editorial split — this is a diagram):

- Section label: "How it works"
- ASCII art architecture diagram from spec, styled with `var(--text-primary)`, `var(--accent)`, `var(--text-muted)` for the different elements. Wrapped in a `font-mono text-xs sm:text-sm` container.

- [ ] **Step 2: Add three-step install section**

`<section class="mb-24 fade-in">`:

- Section label: "Get started"
- Heading: "Up and running in 30 seconds"
- Three install steps, each in a dark inset block with copy button:
  1. `brew install alexcatdad/tap/paw-proxy`
  2. `sudo paw-proxy setup`
  3. `up npm run dev`
- Use same copy button markup pattern as current page (`data-code` attribute).

- [ ] **Step 3: Add Prior Art section**

- Section label: "Prior art"
- Heading: "Standing on giants' shoulders"
- Row of links: mkcert, puma-dev, pow, hotel, caddy — same URLs as current page. Styled as minimal inline links with hover color change, no clay pills.

- [ ] **Step 4: Add footer**

`<footer>` with top border (`var(--border)`):
- Links: GitHub, Releases, Issues, MIT License
- "Built with 🐾 by alexcatdad"
- Close the `</div>` content wrapper

- [ ] **Step 5: Verify complete page structure**

Scroll through entire page. All 6 beats should flow as a story. Architecture diagram, install, prior art, and footer should be below the story.

- [ ] **Step 6: Commit**

```bash
git add docs/index.html
git commit -m "feat(landing): add architecture, install, prior art, and footer"
```

---

## Chunk 4: JavaScript + Build + Polish

### Task 7: Add JavaScript (theme toggle, fade-in, copy buttons)

**Files:**
- Modify: `docs/index.html` (add `<script>` before `</body>`)

- [ ] **Step 1: Add script block**

Before `</body>`, add `<script>` with:

1. **Theme toggle** — same logic as current page but with dark-first default:
   ```js
   const toggle = document.getElementById('theme-toggle');
   const html = document.documentElement;
   // Default to dark unless OS prefers light or user chose light
   if (localStorage.theme === 'light' ||
       (!localStorage.theme && window.matchMedia('(prefers-color-scheme: light)').matches)) {
       html.classList.remove('dark');
   }
   toggle.addEventListener('click', () => {
       html.classList.toggle('dark');
       localStorage.theme = html.classList.contains('dark') ? 'dark' : 'light';
   });
   ```

2. **Fade-in observer** — identical to current page:
   ```js
   const observer = new IntersectionObserver((entries) => {
       entries.forEach(entry => {
           if (entry.isIntersecting) entry.target.classList.add('visible');
       });
   }, { threshold: 0.1, rootMargin: '-40px' });
   document.querySelectorAll('.fade-in').forEach(el => observer.observe(el));
   ```

3. **Copy buttons** — identical to current page (reads `data-code`, writes to clipboard, shows "Copied!").

- [ ] **Step 2: Verify interactivity**

- Click theme toggle: page should switch between dark (#0a0a0a) and light (#fafaf9)
- Scroll down: sections should fade in
- Click copy buttons: should copy text and show "Copied!" feedback
- Reload: theme choice should persist

- [ ] **Step 3: Commit**

```bash
git add docs/index.html
git commit -m "feat(landing): add theme toggle, scroll animations, and copy buttons"
```

### Task 8: Rebuild Tailwind CSS

**Files:**
- Modify: `docs/styles.css` (generated output)

- [ ] **Step 1: Install tailwind if needed and rebuild**

```bash
cd /Users/alex/REPOS/alexcatdad/paw-proxy
npx tailwindcss -i docs/input.css -o docs/styles.css --minify
```

This will scan the new `docs/index.html` for Tailwind utility classes and generate the compiled CSS. Old classes no longer used (clay-related) will be tree-shaken out.

- [ ] **Step 2: Verify page loads correctly with rebuilt CSS**

Open `docs/index.html` in browser. All Tailwind utility classes should resolve. Check:
- `max-w-4xl`, `mx-auto`, `px-6`, `py-20` on content wrapper
- `md:grid-cols-2` activates at 768px+
- `font-mono`, `text-sm`, `text-xs` on terminal blocks
- No unstyled/broken elements

- [ ] **Step 3: Commit**

```bash
git add docs/styles.css docs/tailwind.config.js
git commit -m "chore: rebuild Tailwind CSS with new font stack and page classes"
```

### Task 9: Final review and polish

**Files:**
- Modify: `docs/index.html` (minor tweaks only)

- [ ] **Step 1: Cross-browser check**

Open in Safari and Chrome (at minimum). Verify:
- Dark/light mode renders correctly in both
- Terminal blocks are readable
- Copy buttons work
- Scroll animations trigger
- Mobile layout (use responsive mode) stacks correctly

- [ ] **Step 2: Check meta tags**

View page source and verify:
- `<title>` is `paw-proxy — Stop Fighting Localhost`
- OG title/description match spec
- `theme-color` is `#0a0a0a` / `#fafaf9`
- JSON-LD description is updated
- Favicon still works (paw emoji)

- [ ] **Step 3: Visual polish pass**

Scan the full page for:
- Consistent spacing between beats
- Terminal blocks have consistent padding and border radius
- Prose text is readable (line-height, font-size)
- Orange accent appears in the right places (terminal output URLs, key phrases)
- Architecture diagram alignment

Fix any spacing/sizing issues found.

- [ ] **Step 4: Final commit**

```bash
git add docs/index.html docs/styles.css
git commit -m "feat(landing): polish and finalize HN launch landing page"
```
