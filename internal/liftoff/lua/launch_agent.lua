-- Open one agent window in an existing session. Args: session_id, cwd, label,
-- argv. The argv runs directly (shell = "none"), so no argument is ever
-- interpreted by a shell. Returns the new window so the caller can select it.
local a = rex.args
local _, attach_err = rex.session.attach{session_id=a.session_id}
if attach_err then error(attach_err, 0) end
local window, err = rex.session.new_window{
  session_id = a.session_id,
  focus = false,
  window_label = a.label,
  layout = rex.layout.block{
    flavor = "com.superlogical.terminal.shell",
    options = {cwd=a.cwd, command=a.argv, shell="none"},
  },
}
if err then error(err, 0) end
return {window_id = window.window_id or window.id}
