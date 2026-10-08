-- No window label: Rex then names the tab after the agent's terminal title,
-- which follows the thread name once the agent sets one.
local a=rex.args
local shell=rex.layout.block{flavor="com.superlogical.terminal.shell",options={cwd=a.cwd}}
local agent=rex.layout.block{flavor="com.superlogical.terminal.shell",options={cwd=a.cwd,command={"sh","-lc",a.command}}}
local session,err=rex.call("session.create",{label=a.name,initial_windows={{layout=rex.layout.horizontal(1/3,shell,agent)}}})
if err then error(err,0) end
-- Any failure after creation destroys the session so no half-built one
-- keeps the label.
local ok,focus_err=pcall(function()
  local _,attach_err=rex.session.attach{session_id=session.session_id}
  if attach_err then return attach_err end
  local _,e=rex.session.focus_block{session_id=session.session_id,block_id=session.initial_windows[1].block_ids[2]}
  return e
end)
if not ok or focus_err then
  rex.call("session.destroy",{session_id=session.session_id})
  error(ok and focus_err or tostring(focus_err),0)
end
return session
