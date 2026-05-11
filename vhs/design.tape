# Demo for `kit design` wizard — full run through the form + the new
# simple/detailed/skip Ghostty prompt at the end.
#
# This tape creates a real worktree named `vhs-demo` and leaves it
# behind. Cleanup after rendering:
#
#   kit wash vhs-demo
#
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
Sleep 1500ms

# Screen 1: feature name
Type "vhs-demo"
Sleep 1s
Enter
Sleep 800ms

# Screen 2: Clone local database? — default No
Enter
Sleep 800ms

# Screen 3: Symlink frontend node_modules? — default Yes
Enter
Sleep 800ms

# Screen 4: Track in graphite? — default Yes
Enter
Sleep 1s

# Hide the long pip install / worktree add window — show the final
# "ready" summary + simple/detailed prompt instead.
Hide
Sleep 30s
Show

# Open Ghostty workspace? — pick Skip to keep demo side-effect free
Down
Down
Enter
Sleep 1s

# Start dev servers? — pick Skip (Negative)
Right
Enter
Sleep 1500ms
