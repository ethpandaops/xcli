# UI Package

The `ui` package provides centralized terminal UI utilities for xcli, enabling clean, colorful output with visual progress indicators.

## Overview

This package wraps [pterm](https://github.com/pterm/pterm) to provide consistent styling and terminal output throughout the application. It includes:

- **Styled messages** (success, error, warning, info, headers)
- **Spinners** for long-running operations
- **Progress bars** for multi-step processes
- **Tables** for structured data display
- **Conditional logging** based on verbose mode

## Components

### Styled Output (`ui.go`)

Basic styled message functions for user-facing output:

```go
import "github.com/ethpandaops/xcli/pkg/ui"

// Success messages (green checkmark)
ui.Success("Build complete!")

// Error messages (red X)
ui.Error("Failed to start service")

// Warnings (yellow symbol)
ui.Warning("Port already in use")

// Info messages (cyan arrow)
ui.Info("Starting infrastructure")

// Section headers (bold cyan)
ui.Header("Phase 1: Building Services")

// Banner with border
ui.Banner("Starting Lab Stack")

// Blank line for spacing
ui.Blank()
```

### Spinners (`spinner.go`)

Visual progress indicators for long-running operations:

```go
// Basic spinner
spinner := ui.NewSpinner("Cloning repository")
// ... perform operation ...
spinner.Success("Repository cloned")

// Spinner with error handling
spinner := ui.NewSpinner("Installing dependencies")
if err := doInstall(); err != nil {
    spinner.Fail("Installation failed")
    return err
}
spinner.Success("Dependencies installed")

// Spinner with duration display
spinner := ui.NewSpinner("Building service")
start := time.Now()
// ... build ...
spinner.SuccessWithDuration("Built service", time.Since(start))

// Update spinner text during operation
spinner := ui.NewSpinner("Starting services")
spinner.UpdateText("Waiting for health checks")
// ...
spinner.Success("Services ready")

// Convenience wrapper
err := ui.WithSpinner("Running migrations", func() error {
    return runMigrations()
})
```

### Progress Bars (`progress.go`)

Track progress through multi-step operations:

```go
// Create progress bar
bar := ui.NewProgressBar("Building repositories", 5)

// Increment progress
for _, repo := range repos {
    buildRepo(repo)
    bar.Increment()
}

// Stop when complete
bar.Stop()

// Convenience wrapper
ui.WithProgress("Building", len(items), func(bar *ui.ProgressBar) error {
    for _, item := range items {
        processItem(item)
        bar.Increment()
    }
    return nil
})
```

### Tables (`table.go`)

Display structured data in formatted tables:

```go
// Basic table
headers := []string{"Name", "Value"}
rows := [][]string{
    {"cbt", "/path/to/cbt"},
    {"lab", "/path/to/lab"},
}
ui.Table(headers, rows)

// Service table with colored status
services := []ui.Service{
    {Name: "Lab Frontend", URL: "http://localhost:5173", Status: "running"},
    {Name: "Lab Backend", URL: "http://localhost:8080", Status: "down"},
}
ui.ServiceTable(services)

// Key-value table
data := map[string]string{
    "Mode":    "local",
    "Network": "mainnet",
}
ui.KeyValueTable("Configuration", data)
```

### Conditional Writer (`writer.go`)

Control log output based on verbose mode:

```go
// Create conditional writer
logWriter := ui.NewConditionalWriter(os.Stdout, false) // logs disabled

// Enable/disable based on verbose flag
logWriter.SetEnabled(verbose)

// Use with logrus
log.SetOutput(logWriter)
```

## Usage Guidelines

### When to Use Each Component

**Spinners:**
- Long-running operations (>1 second)
- Git clones, npm/pnpm installs
- Database migrations
- Docker container startups
- Any operation that might appear to "hang"

**Progress Bars:**
- Parallel build operations
- Processing multiple items
- When you want to show X of Y completion

**Tables:**
- Service status listings
- Repository information
- Configuration display
- Any structured data with multiple rows

**Styled Messages:**
- Command completion (Success)
- Error messages (Error)
- Warnings about existing state (Warning)
- Informational messages (Info)
- Section separators (Header, Banner)

### Best Practices

1. **Respect verbose mode**: Only show spinners/progress in non-verbose mode
   ```go
   if !verbose {
       spinner := ui.NewSpinner("Building")
       defer spinner.Success("Built")
   }
   ```

2. **Always stop spinners**: Use defer to ensure cleanup
   ```go
   spinner := ui.NewSpinner("Processing")
   defer spinner.Stop()
   ```

3. **Provide meaningful messages**: Update spinner text to show what's happening
   ```go
   spinner.UpdateText("Installing dependencies")
   // ... install ...
   spinner.UpdateText("Building assets")
   // ... build ...
   ```

4. **Use duration display for builds**: Help users understand performance
   ```go
   spinner.SuccessWithDuration("Built", duration)
   ```

5. **Consistent messaging**: Use the same message patterns across commands

### Verbose Mode Integration

The UI package works seamlessly with xcli's `--verbose` flag:

- **Default mode**: Clean output with spinners, progress bars, and colored messages. Logrus logs are hidden.
- **Verbose mode** (`-v` or `--verbose`): All logrus logs visible, spinners automatically disabled by pterm.

Implementation in main.go:
```go
logWriter := ui.NewConditionalWriter(os.Stdout, false)
log.SetOutput(logWriter)

// Later, after flag parsing
logWriter.SetEnabled(verbose)
```

## Technical Details

### Dependencies

- **pterm v0.12.82**: Modern terminal rendering library
  - Automatic TTY detection
  - Graceful degradation in non-TTY environments
  - Thread-safe rendering

### TTY Detection

pterm automatically detects non-TTY environments (pipes, CI/CD) and disables fancy output, falling back to plain text. This ensures output remains clean in:

- Piped commands: `xcli lab up | tee output.log`
- CI/CD environments: GitHub Actions, Jenkins, etc.
- Non-interactive shells

### Color Scheme

- **Success**: Green (`pterm.FgGreen`)
- **Error**: Red (`pterm.FgRed`)
- **Warning**: Yellow (`pterm.FgYellow`)
- **Info**: Cyan (`pterm.FgCyan`)
- **Headers**: Bold Cyan (`pterm.FgCyan`, `pterm.Bold`)

### Performance

UI operations add minimal overhead (<1ms per call). Spinners are rendered in background goroutines and don't block execution.

## Examples from xcli

### Infrastructure Startup (with health check updates)

```go
spinner := ui.NewSpinner("Starting infrastructure services")

if err := dockerComposeUp(); err != nil {
    spinner.Fail("Failed to start infrastructure")
    return err
}

spinner.UpdateText("Waiting for services to be healthy")

for i := 0; i < maxAttempts; i++ {
    spinner.UpdateText(fmt.Sprintf("Health check %d/%d", i+1, maxAttempts))
    if allHealthy() {
        break
    }
    time.Sleep(2 * time.Second)
}

spinner.Success("Infrastructure started successfully")
```

### Parallel Builds

```go
bar := ui.NewProgressBar("Building repositories", len(repos))

var wg sync.WaitGroup
for _, repo := range repos {
    wg.Add(1)
    go func(r string) {
        defer wg.Done()

        spinner := ui.NewSpinner(fmt.Sprintf("Building %s", r))
        if err := build(r); err != nil {
            spinner.Fail(fmt.Sprintf("Failed: %s", r))
        } else {
            spinner.Success(r)
        }

        bar.Increment()
    }(repo)
}

wg.Wait()
bar.Stop()
```

### Multi-Phase Orchestration

```go
ui.Banner("Starting Lab Stack")

ui.Header("Phase 1: Building Xatu-CBT")
// ... build ...

ui.Header("Phase 2: Starting Infrastructure")
// ... infrastructure ...

ui.Header("Phase 3: Building Services")
// ... builds ...

ui.Blank()
ui.Success("Stack is running!")

services := []ui.Service{
    {Name: "Lab Frontend", URL: "http://localhost:5173", Status: "running"},
    {Name: "Lab Backend", URL: "http://localhost:8080", Status: "running"},
}
ui.Header("Services")
ui.ServiceTable(services)
```

## Maintenance

When adding new UI components:

1. Follow the existing patterns in `style.go` for colors
2. Add proper godoc comments for all exported functions
3. Use pterm components directly, wrapping for consistency
4. Test in both TTY and non-TTY environments
5. Ensure graceful degradation when colors aren't supported
