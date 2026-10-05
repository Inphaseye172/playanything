-- pa-osd.lua — PlayAnything
-- On-screen feedback that makes one-click opening feel instant:
--   * "Opening …" as soon as the window appears (cloud files can take a while)
--   * a note when the file is a cloud placeholder being downloaded
--   * a track list when a file has several audio tracks (press a to cycle)
--   * a clear error if the file could not be opened
-- Script options (set by playanything): pa-cloud=<provider>, pa-cloud-note=<text>
local msg = require 'mp.msg'

local function basename(p) return (p or ""):match("[^/\\]+$") or p end

local function describe_track(t)
    local parts = {}
    if t.lang then parts[#parts+1] = t.lang end
    if t.title then parts[#parts+1] = t.title end
    local tech = {}
    if t.codec then tech[#tech+1] = t.codec end
    if t["demux-channel-count"] then tech[#tech+1] = t["demux-channel-count"] .. "ch" end
    if t["demux-samplerate"] then tech[#tech+1] = math.floor(t["demux-samplerate"] / 1000) .. "kHz" end
    if #tech > 0 then parts[#parts+1] = "(" .. table.concat(tech, " ") .. ")" end
    if t.default then parts[#parts+1] = "[default]" end
    if t.external then parts[#parts+1] = "[external]" end
    return table.concat(parts, " ")
end

local function show_tracks(duration)
    local tracks = mp.get_property_native("track-list", {})
    local audio, subs = {}, {}
    for _, t in ipairs(tracks) do
        if t.type == "audio" then audio[#audio+1] = t
        elseif t.type == "sub" then subs[#subs+1] = t end
    end
    if #audio <= 1 and #subs == 0 then
        mp.osd_message(#audio == 1 and ("Audio: " .. describe_track(audio[1])) or "No audio track", 2)
        return
    end
    local lines = {}
    if #audio > 0 then
        lines[#lines+1] = string.format("Audio tracks (%d) — press a to switch", #audio)
        for i, t in ipairs(audio) do
            lines[#lines+1] = string.format("%s %d: %s", t.selected and "▶" or "  ", i, describe_track(t))
        end
    end
    if #subs > 0 then
        lines[#lines+1] = string.format("Subtitles (%d) — press s to switch", #subs)
        for i, t in ipairs(subs) do
            lines[#lines+1] = string.format("%s %d: %s", t.selected and "▶" or "  ", i, describe_track(t))
        end
    end
    mp.osd_message(table.concat(lines, "\n"), duration or 5)
end

mp.register_script_message("pa-show-tracks", function() show_tracks(6) end)

local current_name = ""

mp.register_event("start-file", function()
    local name = basename(mp.get_property("path", ""))
    current_name = name
    local cloud = mp.get_opt("pa-cloud")
    if cloud and cloud ~= "" then
        mp.osd_message(string.format("Opening %s\n⬇ Downloading from %s — not stored locally yet", name, cloud), 120)
    else
        mp.osd_message("Opening " .. name .. " …", 30)
    end
end)

mp.register_event("file-loaded", function()
    mp.osd_message("", 0)
    local n = 0
    for _, t in ipairs(mp.get_property_native("track-list", {})) do
        if t.type == "audio" then n = n + 1 end
    end
    if n > 1 then mp.add_timeout(0.2, function() show_tracks(5) end) end
end)

mp.register_event("end-file", function(ev)
    if ev.reason == "error" then
        local name = current_name ~= "" and current_name or basename(mp.get_property("path", ""))
        local why = ev.file_error or "unknown error"
        msg.error("could not open " .. name .. ": " .. why)
        mp.osd_message("Could not open " .. name .. "\n" .. why, 8)
    end
end)

-- When audio is switched with a/A/#, show which track is now active.
mp.observe_property("aid", "native", function(_, aid)
    if aid == nil or aid == false then return end
    if mp.get_property_number("playlist-pos", -1) < 0 then return end
    local tracks = mp.get_property_native("track-list", {})
    local count = 0
    for _, t in ipairs(tracks) do if t.type == "audio" then count = count + 1 end end
    if count > 1 then
        for _, t in ipairs(tracks) do
            if t.type == "audio" and t.id == aid then
                mp.osd_message(string.format("Audio %d/%d: %s", aid, count, describe_track(t)), 2)
            end
        end
    end
end)
