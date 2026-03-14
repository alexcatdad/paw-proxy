# Landing Page Redesign — HN Launch

**Date:** 2026-03-14
**Status:** Approved
**Goal:** Redesign docs/index.html so an HN reader understands paw-proxy's value in 10 seconds.

## Context

paw-proxy's current landing page leads with "HTTPS is painful" and uses a claymorphism design with a feature grid. For a Show HN submission, we need a page that tells the real origin story — port conflicts and worktree confusion — in a pragmatic, scannable format.

### Origin Story

Running multiple projects on the same stack (e.g., 3 Next.js apps) causes port conflicts. All expect port 3000; the second silently moves to 3001. OAuth callbacks, auth redirects, and test fixtures are hardcoded to 3000. Combined with git worktrees of the same project, this leads to testing code in the wrong path or against untouched code.

The insight: the problem isn't ports — it's identity. `localhost:3000` doesn't tell you what you're running. Named domains do. And once you have names, HTTPS comes free.

### Target Audience

HN readers. Pragmatic, problem-first. They want to understand the idea, not be sold on it. No social proof yet — this is the launch.

## Design

### Approach: Editorial Split

Two-column layout: prose on the left tells the story, terminal snippets on the right illustrate each beat. Single column (stacked) on mobile.

### Visual Direction: Stark Mono

- **Dark mode (flagship):** Pure black `#0a0a0a` background, high-contrast white `#fafafa` text, orange `#f97316` accent
- **Light mode (alternative):** Warm stone `#fafaf9` background, dark `#1c1917` text, burnt orange `#ea580c` accent
- **Theme toggle:** Preserved. Uses same `dark` class toggle pattern as current page. Rule: use OS preference if no localStorage override exists. If OS preference is light, show light mode. If no OS preference signal, default to dark. Dark is the flagship but we don't force it over explicit OS settings.
- **`theme-color` meta tag:** Update to `#0a0a0a` (dark) and `#fafaf9` (light) to match new palette.
- **Fonts:** System UI stack (`system-ui, -apple-system, sans-serif`) for prose. JetBrains Mono (Google Fonts, already loaded) for code/terminal blocks. Nunito is removed.
- **No decoration:** No claymorphism, no neumorphic shadows, no gradient text, no bouncing paw animation. Content is the design.

### Color Implementation

All theme colors are defined in CSS custom properties in the `<style>` block — not in `tailwind.config.js`. This avoids needing to update the Tailwind config for palette changes. The existing Tailwind `accent` colors (`#F97316`, `#EA580C`) happen to match the spec, so `text-accent` classes still work.

Dark mode CSS variables:
```css
:root {
    --bg-base: #0a0a0a;
    --bg-surface: #141414;
    --bg-terminal: #111111;
    --text-primary: #fafafa;
    --text-secondary: #a3a3a3;
    --text-muted: #525252;
    --border: #262626;
    --accent: #f97316;
}
```

Light mode CSS variables (when `.dark` class is absent — matches current toggle pattern):
```css
:root:not(.dark) {
    --bg-base: #fafaf9;
    --bg-surface: #f5f5f4;
    --bg-terminal: #1c1917;
    --text-primary: #1c1917;
    --text-secondary: #57534e;
    --text-muted: #a8a29e;
    --border: #e7e5e4;
    --accent: #ea580c;
}
```

### Font Implementation

Update `docs/tailwind.config.js` to change the `sans` font family:

```js
fontFamily: {
    sans: ['system-ui', '-apple-system', 'sans-serif'],
    mono: ['JetBrains Mono', 'monospace'],
},
```

Remove Nunito from the Google Fonts `<link>` tag. Keep JetBrains Mono.

After making any changes to `tailwind.config.js` or using new utility classes in the HTML, rebuild:

```bash
npx tailwindcss -i docs/input.css -o docs/styles.css --minify
```

### Narrative Structure

The page tells a story in 6 beats. Each beat is a section with prose (left) and terminal output (right).

#### Beat 1 — Hero

- Headline: **"Stop fighting localhost."**
- Subline: **"Named HTTPS domains for every local dev server. Zero config."**
- CTA: `brew install alexcatdad/tap/paw-proxy` with copy button
- Terminal block:

```
❯ up npm run dev
→ https://myapp.test ✓
```

#### Beat 2 — The Pain: Port Conflicts

- Prose: Three Next.js projects all want port 3000. Second one bumps to 3001. OAuth callback hardcoded to 3000 breaks. Env vars and test fixtures all expect 3000. You've been debugging the wrong app for 20 minutes.
- Terminal block:

```
❯ npm run dev              # project-a
→ localhost:3000

❯ npm run dev              # project-b
⚠ port 3000 in use, using 3001
→ localhost:3001

❯ open http://localhost:3001/auth/callback
✗ OAuth error: redirect_uri_mismatch
  expected: http://localhost:3000/auth/callback
```

#### Beat 3 — The Pain: Worktree Confusion

- Prose: Git worktrees of the same project. Both are "myapp" on localhost. Which tab is which? You just spent 10 minutes testing code that hasn't changed because you're hitting the wrong instance.
- Terminal block:

```
# ~/myapp (main branch)
❯ npm run dev → localhost:3000

# ~/myapp-feat-auth (feature branch)
❯ npm run dev → localhost:3001

# two browser tabs, both say "MyApp"
# ...which branch is this again?
# you refresh, nothing changed
# because you're hitting the wrong one
```

#### Beat 4 — The Insight

- Prose: **"The problem isn't ports. It's identity."** localhost:3000 doesn't tell you what you're running. A named domain does. Once you have names, HTTPS comes free — real certs, no browser warnings, OAuth just works.
- Terminal block:

```
# ports tell you nothing
localhost:3000  → ???
localhost:3001  → ???
localhost:3002  → ???

# names tell you everything
myapp.test           → myapp (main)
myapp-feat-auth.test → myapp (feature branch)
api.shop.test        → shop's API service
```

#### Beat 5 — The Solution

- Prose: **"Just prefix with `up`."** paw-proxy gives every dev server a named HTTPS domain. Handles DNS, certificates, port allocation, worktree conflicts. Zero config files, zero nginx, zero mkcert.
- Features woven into prose (not a separate grid):
  - Auto SSL (trusted certs on-the-fly)
  - WebSocket/HMR (hot reload just works)
  - Docker Compose (auto-discovers services → `frontend.shop.test`, `api.shop.test`)
  - Live dashboard at `_paw.test`
  - Worktree-aware naming (automatic fallback when domain collides)
- Terminal block:

```
# any dev server
❯ up npm run dev
→ https://myapp.test ✓

# docker compose — every service gets a domain
❯ up docker compose up
→ https://frontend.shop.test ✓
→ https://api.shop.test ✓

# worktrees — automatic conflict resolution
❯ up npm run dev        # from ~/myapp-feat-auth
⚠ myapp.test in use
→ https://myapp-feat-auth.test ✓
```

#### Beat 6 — How It Works + Install

- Architecture diagram: Uses the same ASCII art structure as the current page. Styled with CSS custom properties (`var(--text-primary)`, `var(--accent)`, `var(--text-muted)`) so it automatically adapts to both themes. No hardcoded color classes.

```
┌─────────────┐     ┌──────────────────┐     ┌─────────────┐
│   Browser   │────▶│   paw-proxy      │────▶│  Dev Server │
│             │     │   port 443       │     │  dynamic    │
└─────────────┘     └──────────────────┘     └─────────────┘
       │                     │
       │              ┌──────┴──────┐
       │              │  DNS Server │  *.test → 127.0.0.1
       │              │  port 9353  │
       │              └─────────────┘
       │
       └──── https://myapp.test ✓ Trusted SSL
```

- Three-step install:
  1. `brew install alexcatdad/tap/paw-proxy`
  2. `sudo paw-proxy setup`
  3. `up npm run dev`
- Each step has a copy button (keep existing copy button JS).
- Prior art section: mkcert, puma-dev, pow, hotel, caddy (keep existing links).
- Footer: GitHub, Releases, Issues, MIT License, "Built with 🐾 by alexcatdad"

### SEO & Meta Tags

Updated text for all meta tags:

- **Page title:** `paw-proxy — Stop Fighting Localhost`
- **Meta description:** `Named HTTPS domains for every local dev server. No more port conflicts, no more localhost confusion. Just prefix with "up".`
- **OG title:** `paw-proxy — Stop Fighting Localhost`
- **OG description:** `Named HTTPS domains for every local dev server. No more port conflicts, no more localhost confusion. Just prefix with "up".`
- **Twitter title/description:** Same as OG.
- **JSON-LD description:** `Named HTTPS domains for local development. Stop fighting port conflicts — get https://myapp.test working in seconds with zero configuration.`

Keep all other meta unchanged: canonical URL, OG image, twitter:card, favicon, llms.txt links.

### What Changes

| Element | Current | New |
|---------|---------|-----|
| Design system | Claymorphism (clay cards, neumorphic shadows) | Flat, high-contrast Stark Mono |
| Headline | "Zero-config HTTPS for local macOS development" | "Stop fighting localhost." |
| Subline | "Get https://myapp.test working in seconds." | "Named HTTPS domains for every local dev server. Zero config." |
| Story | Leads with "HTTPS is painful" | Leads with port conflicts and worktree confusion |
| Features | 8-card grid section | Woven into the solution narrative (Beat 5) |
| Font (body) | Nunito (Google Fonts) | System UI stack |
| Font (code) | JetBrains Mono | JetBrains Mono (unchanged) |
| Layout | Single column, card-based | Two-column editorial split (prose \| terminal) |
| Animations | Bouncing paw, fade-in on scroll | Fade-in on scroll only |
| Dark/light | Toggle with clay button | Toggle with minimal button |
| Colors | Sand/slate palette via CSS vars + Tailwind config | Black/stone palette via CSS vars; Tailwind accent colors unchanged |
| Meta tags | "Zero-config HTTPS for Local Development" | "Stop Fighting Localhost" |

### What Stays

- Copy-to-clipboard buttons on install commands
- Theme toggle (light/dark) with localStorage + OS preference
- IntersectionObserver fade-in animations
- Paw emoji favicon
- JSON-LD structured data (updated description)
- llms.txt links
- Prior art / credits section
- Footer links (GitHub, Releases, Issues, License)
- Tailwind CSS (compiled via styles.css)
- Existing `accent` colors in tailwind.config.js (`#F97316` / `#EA580C`) — they match the new palette

## Technical Notes

- **File:** `docs/index.html` — full rewrite of existing file
- **Tailwind config:** `docs/tailwind.config.js` — update `fontFamily.sans` only (remove Nunito, use system-ui stack). All other config stays.
- **CSS:** Tailwind utilities in HTML + `<style>` block for CSS custom properties (theme colors) and minimal custom styles (terminal dots, cursor blink animation)
- **JS:** Vanilla JS in `<script>` block — theme toggle, IntersectionObserver, copy buttons. No framework.
- **Responsive:** Default layout is single-column stacked (mobile-first). Two-column editorial grid activates at `md:` breakpoint (768px) and above. Within each beat, prose block appears first, terminal block below on mobile.
- **Google Fonts link:** Update to load only JetBrains Mono (remove Nunito from the URL).
- **Build step:** After editing `docs/index.html` or `docs/tailwind.config.js`, rebuild CSS:
  ```bash
  npx tailwindcss -i docs/input.css -o docs/styles.css --minify
  ```
