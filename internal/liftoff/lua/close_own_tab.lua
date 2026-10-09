-- Close the tab holding this block when it holds nothing else and is not the
-- session's last tab. Args: session_id, block_id. Returns true when closed.
local a = rex.args
local _, attach_err = rex.session.attach{session_id=a.session_id}
if attach_err then error(attach_err, 0) end
local view, view_err = rex.session.view{session_id=a.session_id}
if view_err then error(view_err, 0) end
local blocks, blocks_err = rex.session.list_blocks{session_id=a.session_id}
if blocks_err then error(blocks_err, 0) end
local window_id
for _, block in ipairs(blocks.blocks or {}) do
  if block.block_id == a.block_id then window_id = block.window_id end
end
if not window_id or #(view.windows or {}) < 2 then return false end
for _, block in ipairs(blocks.blocks or {}) do
  if block.window_id == window_id and block.block_id ~= a.block_id then return false end
end
local _, close_err = rex.session.close_window{session_id=a.session_id, window_id=window_id}
if close_err then error(close_err, 0) end
return true
