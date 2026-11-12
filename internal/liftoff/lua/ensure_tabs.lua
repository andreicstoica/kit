-- Add the listed tabs to a session in one request. Args: session_id, cwd,
-- tabs = {{label=, command=}}. Stops at the first failure; earlier tabs stay.
-- An empty label keeps Rex's default window title.
local a = rex.args
local _, attach_err = rex.session.attach{session_id=a.session_id}
if attach_err then error(attach_err, 0) end
for _, tab in ipairs(a.tabs or {}) do
  local options = {cwd=a.cwd}
  if tab.command and tab.command ~= "" then
    options.command = {"sh", "-lc", tab.command}
    options.shell = "none"
  end
  local request = {
    session_id = a.session_id,
    focus = false,
    layout = rex.layout.block{flavor="com.superlogical.terminal.shell", options=options},
  }
  if tab.label and tab.label ~= "" then
    request.window_label = tab.label
  end
  local _, err = rex.session.new_window(request)
  local name = tab.label
  if not name or name == "" then name = "default" end
  if err then error("create " .. name .. " tab: " .. err, 0) end
end
return true
