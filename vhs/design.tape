# Demo for `kit design` wizard (intro screen only — aborts before
# creating a real worktree).
# Render with: vhs vhs/design.tape  → produces vhs/design.gif

Output vhs/design.gif

Set Shell "zsh"
Set FontSize 16
Set Width 1200
Set Height 800
Set Theme "ayu_light"
Set Padding 24
Set TypingSpeed 50ms

Type "kit design"
Sleep 500ms
Enter
Sleep 2s

# Show the name input
Type "demo-feature"
Sleep 1500ms

# Abort before advancing to keep the demo side-effect free.
Ctrl+C
Sleep 500ms
