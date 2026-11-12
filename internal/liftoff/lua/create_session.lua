-- Create a session with one window per tab, published atomically: a failure
-- leaves no half-built session. Args: name, cwd, tabs = {{label=, command=}}.
-- An empty label keeps Rex's default window title.
local a = rex.args
local windows = {}
for _, tab in ipairs(a.tabs or {}) do
  local options = {cwd=a.cwd}
  if tab.command and tab.command ~= "" then
    options.command = {"sh", "-lc", tab.command}
    options.shell = "none"
  end
  local window = {layout = rex.layout.block{flavor="com.superlogical.terminal.shell", options=options}}
  if tab.label and tab.label ~= "" then
    window.window_label = tab.label
  end
  table.insert(windows, window)
end
local session, err = rex.call("session.create", {label=a.name, initial_windows=windows})
if err then error(err, 0) end
return session
