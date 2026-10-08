local source = debug.getinfo(1, "S").source
-- Rex may report a raw path; stock Lua prefixes file sources with @.
if source:sub(1, 1) == "@" then source = source:sub(2) end
local directory = source:match("^(.*)/")
assert(directory and directory:sub(1, 1) == "/", "Load Kit actions from an absolute file path")
local runner = directory .. "/rex-action"
local home = os.getenv("HOME")
local kit = os.getenv("KIT_REX_BIN") or home .. "/.local/bin/kit"

local function options(cwd, verb, session_id)
  return {
    cwd = cwd,
    command = {
      "/usr/bin/env",
      "KIT_REX_BIN=" .. kit,
      "KIT_STATE_DIR=" .. (os.getenv("KIT_STATE_DIR") or home .. "/.config/kit"),
      "KIT_RUN_DIR=" .. (os.getenv("KIT_RUN_DIR") or home .. "/.config/kit/run"),
      "KIT_WORKSPACE_BACKEND=rex",
      "/bin/zsh", "-lic", "exec /bin/sh \"$1\" \"$2\" \"$3\"", "kit-palette", runner, verb, session_id,
    },
    shell = "none",
    exit = {on_completion=true},
  }
end

local actions = {
  {"design", "Kit: New workspace", "Create a workspace with Kit's interactive wizard"},
  {"restart", "Kit: Restart services", "Run Kit's restart CLI with visible progress"},
}

for _, entry in ipairs(actions) do
  local verb, title, description = entry[1], entry[2], entry[3]
  rex.action{
    name = "kit_" .. verb:gsub("-", "_"),
    title = title,
    description = description,
    category = "Kit",
    keywords = "kit workspace " .. verb,
    args = {},
    run = function(ctx)
      if not ctx.session_id then error("Select a Rex session first", 0) end
      local cwd = os.getenv("HOME")
      if ctx.block_id then
        local terminal = rex.block.call("com.superlogical.terminal", "pwd", {session_id=ctx.session_id, block_id=ctx.block_id})
        if terminal and terminal.pwd and terminal.pwd ~= "" then cwd = terminal.pwd end
      end
      local window, err = rex.session.new_window{
        session_id = ctx.session_id,
        window_label = title,
        focus = true,
        layout = rex.layout.block{
          flavor = "com.superlogical.terminal.shell",
          options = options(cwd, verb, ctx.session_id),
        },
      }
      if err then error(err, 0) end
      if ctx.origin == "palette" or ctx.origin == "key" then
        rex.client.queue("session.select", {session_id=ctx.session_id, window_id=window.window_id})
      end
      return window
    end,
  }
end
