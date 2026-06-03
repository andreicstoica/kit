# Demo for the `kit design` side-panel animations (the preview gallery).
# Runs dev/animgallery, which tiles every animation at once — no worktree
# side effects. Render with: vhs vhs/anim.tape  → produces vhs/anim.gif

Output "vhs/anim.gif"

Set Shell "zsh"
Set FontSize 16
Set Width 1600
Set Height 1000
Set Theme "ayu_light"
Set Padding 24
Set TypingSpeed 50ms

# Off-camera: build the gallery binary, tidy the prompt, clear the build
# output so the gif opens straight on the gallery.
Hide
Type "go build -o /tmp/kit-animgallery ./dev/animgallery"
Enter
Sleep 12s
Type "PROMPT='%% '"
Enter
Type "clear"
Enter
Show

# Run it and let every animation cycle through its loop a few times.
Type "/tmp/kit-animgallery"
Enter
Sleep 10s

# Quit cleanly.
Type "q"
Sleep 500ms
