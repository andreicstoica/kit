package liftoff

// Atomic publication prevents a failed agent-pane creation from leaving a half-built session.
const rexSimpleSessionLua = `local a=rex.args
local shell=rex.layout.block{flavor="com.superlogical.terminal.shell",options={cwd=a.cwd}}
local agent=rex.layout.block{flavor="com.superlogical.terminal.shell",label="claude",options={cwd=a.cwd,command={"sh","-lc",a.command}}}
local session,err=rex.call("session.create",{label=a.name,initial_windows={{window_label="shell",layout=rex.layout.horizontal(1/3,shell,agent)}}})
if err then error(err,0) end
local _,attach_err=rex.session.attach{session_id=session.session_id}
local focus_err=attach_err
if not focus_err then
  local focused
  focused,focus_err=rex.session.focus_block{session_id=session.session_id,block_id=session.initial_windows[1].block_ids[2]}
end
if focus_err then
  rex.call("session.destroy",{session_id=session.session_id})
  error(focus_err,0)
end
return session`
