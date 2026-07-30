# CSS tokens

## Files

| File | Role |
| --- | --- |
| `tokens.css` | design tokens only. No selectors beyond `:root`. |
| `app.css` | reset, element defaults, base components. `@import`s `tokens.css`. |
| `styleguide.html` | visual reference. Open directly in a browser, no server needed. |

A page links **`app.css` only** — it pulls in `tokens.css` itself.

> **When wiring the Go static handler:** embed the whole CSS directory, not
> `app.css` alone. `app.css` requests `tokens.css` as a sibling URL, so a
> single-file `embed` returns 404 for it and the page renders completely
> unstyled with no error anywhere. `//go:embed assets/src/css` is the safe form.

`tokens.css` is the framework-independent visual foundation. Import it before
page or component styles:

```css
@import "./tokens.css";

body {
  color: var(--color-text);
  background: var(--color-canvas);
  font-family: var(--font-sans);
}
```

## Use semantic tokens

Components should use role-based tokens, not palette values:

```css
.action {
  min-height: var(--control-height-md);
  padding-inline: var(--space-4);
  color: var(--color-on-action);
  background: var(--color-action);
  border-radius: var(--radius-md);
  transition: background var(--duration-fast) var(--ease-standard);
}

.action:hover {
  background: var(--color-action-hover);
}

.action:focus-visible {
  outline: var(--focus-ring-width) solid var(--color-focus);
  outline-offset: var(--focus-ring-offset);
}
```

The intended text/background combinations meet WCAG AA contrast for normal text.
Keep semantic pairs together when customizing status colors.

## Add a brand

Override accent primitives after importing the file. Existing component styles
will inherit the new brand through semantic tokens:

```css
:root {
  --accent-50: #faf5ff;
  --accent-100: #f3e8ff;
  --accent-300: #d8b4fe;
  --accent-400: #c084fc;
  --accent-600: #9333ea;
  --accent-700: #7e22ce;
  --accent-800: #6b21a8;
  --accent-950: #3b0764;
}
```

Recheck contrast after replacing colors. Do not rename semantic tokens to match
a specific brand or visual treatment.

## Themes and accessibility

With no attribute, the system color preference is used. Set `data-theme="light"`
or `data-theme="dark"` on `<html>` for an explicit user choice. Persist that
choice in the application when a theme control is added.

Motion durations collapse when the user requests reduced motion. Interactive
components should use the duration tokens and retain a visible `:focus-visible`
indicator. The minimum touch-target token is 44px.

This foundation intentionally contains no reset, utility classes, or UI
components. Add those only when a real interface establishes their needs.
