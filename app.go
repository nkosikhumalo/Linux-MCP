// Package main
//
// app.go — WHAT BELONGS HERE (Wails bridge)
// ----------------------------------------
// This is the Go ↔ JavaScript bridge for the desktop UI.
// Wails exposes methods on a struct to the frontend as:
//   window.go.main.App.MethodName(...)
//
// YOU SHOULD PUT HERE:
//   - type App struct { ... }  (holds router, ctx, config)
//   - NewApp() *App
//   - Startup(ctx)  — Wails lifecycle hook
//   - Chat(message string) (reply, trace, error)  — called from JS
//   - ListTools() — optional helper for the UI
//
// YOU SHOULD NOT PUT HERE:
//   - OpenRouter HTTP details → pipeline/client.go
//   - Tool implementations    → mcp/handlers.go
//   - HTML/CSS/JS             → frontend/
//
// LEARNING HINT:
//   Keep App thin: receive UI call → call pipeline.Router → return result.
//   Fill this AFTER pipeline/router.go has a working Run(...).
//
package main
