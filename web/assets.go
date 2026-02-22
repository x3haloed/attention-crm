package web

import "embed"

// StaticFS embeds compiled frontend assets (Tailwind output, etc.).
//
// Source files live in web/src; compiled output should be written to web/static.
//
//go:embed static/*
var StaticFS embed.FS
