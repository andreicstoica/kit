-- Add the listed tabs to a session in one request. Args: session_id, cwd,
-- tabs = {{label=, command=}}. Stops at the first failure; earlier tabs stay.
local a = rex.args
local _, attach_err = rex.session.attach{session_id=a.session_id}
if attach_err then error(attach_err, 0) end
for _, tab in ipairs(a.tabs or {}) do
  local options = {cwd=a.cwd}
  if tab.command and tab.command ~= "" then
    options.command = {"sh", "-lc", tab.command}
    options.shell = "none"
  end
  local _, err = rex.session.new_window{
    session_id = a.session_id,
    window_label = tab.label,
    focus = false,
    layout = rex.layout.block{flavor="com.superlogical.terminal.shell", options=options},
  }
  if err then error("create " .. tab.label .. " tab: " .. err, 0) end
end
return true
