-- pa-autoload.lua — PlayAnything
-- When a single local file is opened, add its folder siblings (video, audio,
-- images, camera RAW) to the playlist in natural order, so < / > (or PgUp /
-- PgDn) step through a camera card or a render folder like an image viewer.
local utils = require 'mp.utils'
local msg = require 'mp.msg'

local EXTS = {}
for _, e in ipairs({
    -- video
    "mkv","mk3d","webm","mp4","m4v","mov","qt","3gp","3g2","f4v","avi","wmv","asf","flv","ogv","ogm","ogx",
    "rm","rmvb","divx","dv","mpg","mpeg","mpe","m1v","m2v","vob","evo","ts","m2ts","mts","tp","trp","mxf",
    "gxf","y4m","nut","wtv","ivf","obu","h264","264","h265","265","hevc","avc","av1","mjpg","mjpeg","dav","amv",
    -- audio
    "mp3","mp2","m4a","m4b","aac","ac3","eac3","dts","dtshd","thd","mlp","truehd","flac","wav","w64","rf64",
    "bwf","aiff","aif","aifc","ogg","oga","opus","spx","wma","wv","ape","tta","tak","mka","mpc","au","caf",
    "amr","dsf","dff","alac","shn",
    -- images
    "jpg","jpeg","jpe","jfif","png","apng","gif","bmp","webp","tif","tiff","tga","pnm","pbm","pgm","ppm","pam",
    "pcx","psd","heic","heif","hif","avif","jxl","jp2","j2k","exr","hdr","dds","dpx","sgi","xbm","xpm","ico","qoi",
    -- camera raw (shown via pa-rawhook.lua)
    "arw","srf","sr2","cr2","cr3","crw","nef","nrw","dng","gpr","raf","orf","rw2","rwl","pef","ptx","srw","3fr",
    "fff","iiq","mef","mos","mrw","erf","kdc","dcr","k25","x3f","rwz",
}) do EXTS[e] = true end

local MAX_ENTRIES = 5000
local done_for = nil

local function ext_of(name)
    local e = name:match("^.+%.([^.]+)$")
    return e and e:lower() or ""
end

-- natural sort key: split digits so clip2 < clip10
local function natkey(s)
    local parts = {}
    for text, num in s:lower():gmatch("([^%d]*)(%d*)") do
        parts[#parts+1] = text
        if num ~= "" then parts[#parts+1] = tonumber(num) end
    end
    return parts
end

local function natless(a, b)
    local ka, kb = natkey(a), natkey(b)
    for i = 1, math.max(#ka, #kb) do
        local x, y = ka[i], kb[i]
        if x == nil then return true end
        if y == nil then return false end
        if type(x) ~= type(y) then return type(x) == "number" end
        if x ~= y then return x < y end
    end
    return false
end

local function split(path)
    local dir, file = utils.split_path(path)
    return dir, file
end

local function autoload()
    if mp.get_opt("pa-autoload") == "no" then return end
    if mp.get_property_number("playlist-count", 0) ~= 1 then return end
    local path = mp.get_property("path", "")
    if path == "" or path:find("://") then return end
    if done_for == path then return end
    done_for = path

    local dir, file = split(path)
    if dir == "" then dir = "." end
    local files = utils.readdir(dir, "files")
    if not files then return end

    local list = {}
    for _, f in ipairs(files) do
        if f:sub(1, 1) ~= "." and EXTS[ext_of(f)] then list[#list+1] = f end
    end
    if #list <= 1 then return end
    table.sort(list, natless)

    local idx
    for i, f in ipairs(list) do if f == file then idx = i break end end
    if not idx then return end

    local added = 0
    for i, f in ipairs(list) do
        if i ~= idx then
            mp.commandv("loadfile", utils.join_path(dir, f), "append")
            added = added + 1
            if added >= MAX_ENTRIES then break end
        end
    end
    -- Current file is entry 0; move it into its natural position.
    local count = mp.get_property_number("playlist-count", 1)
    local target = idx -- entries before it = idx-1, so it should land at index idx-1 => move before entry idx
    if target > count then target = count end
    if target > 1 then mp.commandv("playlist-move", 0, target) end
    msg.info(string.format("autoload: %d sibling files from %s", added, dir))
end

mp.register_event("file-loaded", autoload)
mp.register_event("end-file", function() done_for = nil end)
