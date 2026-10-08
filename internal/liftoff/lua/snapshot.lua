-- Read-only snapshot. Args: session_id (limit to one session), status (add
-- process and program_status records). Attaching is required for session reads;
-- the script's connection ends when it exits.
local a = rex.args
local result = {sessions={}}
local list, err = rex.call("session.list")
if err then error(err, 0) end

local function program_name(p)
  local name = p.argv0
  if type(name) ~= "string" or name == "" then name = p.name end
  if type(name) ~= "string" then return "unknown" end
  return (name:gsub("^%-", ""):match("[^/]*$"))
end

for _, s in ipairs(list.sessions or {}) do
  if not a.session_id or a.session_id == s.session_id then
    -- A session that vanishes mid-read must not fail a fleet-wide snapshot.
    local function fail(err)
      if a.session_id then error(err, 0) end
      return true
    end
    local _, attach_err = rex.session.attach{session_id=s.session_id}
    local v, e, b, be
    if not attach_err then
      v, e = rex.session.view{session_id=s.session_id}
      b, be = rex.session.list_blocks{session_id=s.session_id}
    end
    local skip = false
    if attach_err then skip = fail(attach_err)
    elseif e then skip = fail(e)
    elseif be then skip = fail(be) end
    if not skip then
      local session = {session_id=s.session_id, label=s.label, windows={}, detached_blocks={}}
      local by_window = {}
      for _, block in ipairs(b.blocks or {}) do
        if a.status and block.creator_name == "com.superlogical.terminal" then
          block.program_status = rex.block.call(block.creator_name, "program_status", {session_id=s.session_id, block_id=block.block_id})
        end
        if block.window_id then
          by_window[block.window_id] = by_window[block.window_id] or {}
          table.insert(by_window[block.window_id], block)
        else
          table.insert(session.detached_blocks, block)
        end
      end
      for _, w in ipairs(v.windows or {}) do
        local window = {window_id=w.window_id, label=w.label, active=w.active, blocks={}}
        for _, block in ipairs(by_window[w.window_id] or {}) do
          block.status = "unknown"
          if a.status and block.creator_name and block.block_id then
            local process = rex.block.call(block.creator_name, "process", {session_id=s.session_id, block_id=block.block_id})
            if process then
              if process.foreground then block.status = program_name(process.foreground)
              elseif process.child then block.status = program_name(process.child)
              elseif process.last_exit then block.status = "exited (" .. tostring(process.last_exit.exit_code) .. ")" end
            end
          end
          table.insert(window.blocks, block)
        end
        table.insert(session.windows, window)
      end
      table.insert(result.sessions, session)
    end
  end
end
return result
