-- pa-rawhook.lua — PlayAnything
-- Lets mpv "play" camera RAW photos (ARW, NEF, CR2, CR3, DNG, RAF, ORF, RW2,
-- PEF, ...). When such a file is about to load, we ask the playanything
-- binary to extract the camera's embedded JPEG preview (milliseconds) and swap
-- the stream mpv opens, applying the EXIF rotation. This runs for every
-- playlist entry, so stepping through a folder of RAWs with < and > works.
--
-- Script option (set by playanything):  --script-opts=pa-exe=<path to playanything>
local msg = require 'mp.msg'

local RAW_EXTS = {
    arw=true, srf=true, sr2=true, cr2=true, cr3=true, crw=true, nef=true, nrw=true,
    dng=true, gpr=true, raf=true, orf=true, rw2=true, rwl=true, pef=true, ptx=true,
    srw=true, ["3fr"]=true, fff=true, iiq=true, mef=true, mos=true, mrw=true, erf=true,
    kdc=true, dcr=true, k25=true, x3f=true, raw=true, rwz=true,
}

local function ext_of(path)
    local e = path:match("^.+%.([^./\\]+)$")
    return e and e:lower() or ""
end

local function exe()
    local e = mp.get_opt("pa-exe")
    if e and e ~= "" then return e end
    return "playanything"
end

mp.add_hook("on_load", 50, function()
    local path = mp.get_property("stream-open-filename", "")
    if path == "" or path:find("://") then return end
    if not RAW_EXTS[ext_of(path)] then return end

    local res = mp.command_native({
        name = "subprocess",
        args = { exe(), "raw-preview", path },
        capture_stdout = true,
        capture_stderr = true,
        playback_only = false,
    })
    if res.status ~= 0 or not res.stdout or res.stdout == "" then
        msg.warn("raw-preview failed for " .. path .. ": " .. tostring(res.stderr))
        mp.osd_message("PlayAnything: no embedded preview in " .. path:match("[^/\\]+$"), 6)
        return
    end
    -- stdout: line 1 = preview JPEG path, line 2 = clockwise rotation in degrees
    local jpg, rot = res.stdout:match("^([^\r\n]+)[\r\n]+(%d+)")
    if not jpg then jpg = res.stdout:match("^([^\r\n]+)") end
    if not jpg then return end
    mp.set_property("stream-open-filename", jpg)
    if rot and rot ~= "0" then
        mp.set_property("file-local-options/video-rotate", rot)
    end
    msg.info("RAW preview: " .. path .. " -> " .. jpg .. " rotate=" .. tostring(rot))
end)
