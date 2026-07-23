---
version: alpha
name: Comicify Video Translator Workspace
description: Dense, dark desktop workspace for reviewing and translating video dialogue.
colors:
  canvas: "#0f0c1b"
  surface: "#181428"
  surface-raised: "#231c38"
  border: "#3b3254"
  text: "#e2e8f0"
  text-muted: "#94a3b8"
  primary: "#a855f7"
  accent: "#ec4899"
  success: "#22c55e"
typography:
  title: { fontFamily: Inter, fontSize: 22px, fontWeight: 800, lineHeight: 1.1 }
  body: { fontFamily: Inter, fontSize: 14px, fontWeight: 400, lineHeight: 1.45 }
  label: { fontFamily: Inter, fontSize: 13px, fontWeight: 600, lineHeight: 1.2 }
rounded:
  sm: 6px
  md: 8px
  lg: 12px
spacing:
  xs: 4px
  sm: 8px
  md: 12px
  lg: 16px
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.text}"
    rounded: "{rounded.sm}"
    padding: 8px
  panel:
    backgroundColor: "{colors.surface}"
    rounded: "{rounded.md}"
    padding: "{spacing.lg}"
---

# Comicify Video Translator

## Overview

Desktop workspace for a single user reviewing video dialogue. The opening state prioritizes a clear first action and keeps scan settings plus the glossary visible without scrolling.

## Colors

Use the existing dark violet surfaces with purple for primary interaction and pink only for scan emphasis. Status colors remain semantic.

## Typography

Use the existing Inter family. Numeric status and timestamps use tabular figures where possible.

## Layout

The workspace has a two-panel desktop grid. Before a video is selected, the player is a compact 16:9 onboarding state and video-dependent controls are hidden. Once selected, the player expands to available panel height.

## Elevation & Depth

Use tonal surface changes and one-pixel violet borders. Avoid extra decorative shadows.

## Shapes

Panels use 8px corners; controls use 6px corners; status badges may be pill-shaped.

## Components

The empty player states the next action. Video-dependent actions are unavailable until a video is loaded rather than visually competing with the first action.

## Do's and Don'ts

- Do keep the first action visible at launch.
- Do preserve the current scan and glossary controls.
- Don't hide reviewed dialogue after a video is loaded.
- Don't let the empty player consume the whole workspace.
