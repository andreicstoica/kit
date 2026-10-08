-- Create a session with one window per tab, published atomically: a failure
-- leaves no half-built session. Args: name, cwd, tabs = {{label=, command=}}.
local a = rex.args
local windows = {}
for _, tab in ipairs(a.tabs or {}) do
  local options = {cwd=a.cwd}
  if tab.command and tab.command ~= "" then
    options.command = {"sh", "-lc", tab.command}
    options.shell = "none"
  end
  table.insert(windows, {
    window_label = tab.label,
    layout = rex.layout.block{flavor="com.superlogical.terminal.shell", options=options},
  })
end
local session, err = rex.call("session.create", {label=a.name, initial_windows=windows})
if err then error(err, 0) end
return session
