// Package engine is PlayAnything's embedded playback engine: the libmpv
// client library (FFmpeg demuxing/decoding + libplacebo GPU rendering) loaded
// at runtime into our own process, with no cgo and no external mpv program.
//
// PlayAnything owns the window, the user interface and the behaviour; libmpv
// provides the codecs and the renderer the same way every serious player
// (VLC, Plex, Jellyfin, Chrome) relies on FFmpeg rather than writing its own
// H.264/HEVC/AV1 decoders.
//
// The binding covers exactly what the player needs: options, properties,
// commands, property observation, load hooks (for camera-RAW previews) and the
// event pump. Struct layouts and enum values follow mpv/client.h for client
// API 2.x (mpv 0.35+).
package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Format mirrors mpv_format.
type Format int32

const (
	FormatNone   Format = 0
	FormatString Format = 1
	FormatFlag   Format = 3
	FormatInt64  Format = 4
	FormatDouble Format = 5
)

// EventID mirrors mpv_event_id.
type EventID int32

const (
	EventNone            EventID = 0
	EventShutdown        EventID = 1
	EventLogMessage      EventID = 2
	EventGetPropertyRepl EventID = 3
	EventCommandReply    EventID = 5
	EventStartFile       EventID = 6
	EventEndFile         EventID = 7
	EventFileLoaded      EventID = 8
	EventIdle            EventID = 11
	EventClientMessage   EventID = 16
	EventVideoReconfig   EventID = 17
	EventAudioReconfig   EventID = 18
	EventSeek            EventID = 20
	EventPlaybackRestart EventID = 21
	EventPropertyChange  EventID = 22
	EventQueueOverflow   EventID = 24
	EventHook            EventID = 25
)

// End-file reasons (mpv_end_file_reason).
const (
	EndEOF      = 0
	EndStop     = 2
	EndQuit     = 3
	EndError    = 4
	EndRedirect = 5
)

func (e EventID) String() string {
	switch e {
	case EventShutdown:
		return "shutdown"
	case EventLogMessage:
		return "log-message"
	case EventStartFile:
		return "start-file"
	case EventEndFile:
		return "end-file"
	case EventFileLoaded:
		return "file-loaded"
	case EventIdle:
		return "idle"
	case EventClientMessage:
		return "client-message"
	case EventVideoReconfig:
		return "video-reconfig"
	case EventAudioReconfig:
		return "audio-reconfig"
	case EventSeek:
		return "seek"
	case EventPlaybackRestart:
		return "playback-restart"
	case EventPropertyChange:
		return "property-change"
	case EventHook:
		return "hook"
	}
	return fmt.Sprintf("event-%d", int32(e))
}

// Event is a decoded libmpv event.
type Event struct {
	ID            EventID
	Error         int32
	ReplyUserdata uint64

	// EventPropertyChange
	Name   string
	Format Format
	Str    string  // FormatString
	Flag   bool    // FormatFlag
	Int    int64   // FormatInt64
	Double float64 // FormatDouble
	// FormatNone means the property is unavailable (e.g. no file loaded).

	// EventHook
	HookID uint64

	// EventEndFile
	EndReason       int32
	EndError        int32
	PlaylistEntryID int64

	// EventLogMessage
	LogPrefix, LogLevel, LogText string

	// EventClientMessage
	Args []string
}

// Raw C structs (client.h, 64-bit layout).
type cEvent struct {
	eventID       int32
	err           int32
	replyUserdata uint64
	data          unsafe.Pointer
}

type cEventProperty struct {
	name   *byte
	format int32
	_      int32
	data   unsafe.Pointer
}

type cEventHook struct {
	name *byte
	id   uint64
}

type cEventEndFile struct {
	reason          int32
	err             int32
	playlistEntryID int64
	insertID        int64
	insertNum       int32
	_               int32
}

type cEventLogMessage struct {
	prefix   *byte
	level    *byte
	text     *byte
	logLevel int32
	_        int32
}

type cEventClientMessage struct {
	numArgs int32
	_       int32
	args    **byte
}

// Library is the loaded libmpv.
type Library struct {
	Path    string
	Version uint32 // mpv_client_api_version()

	clientAPIVersion  func() uint32
	create            func() uintptr
	initialize        func(h uintptr) int32
	terminateDestroy  func(h uintptr)
	setOptionString   func(h uintptr, name, value string) int32
	commandString     func(h uintptr, cmd string) int32
	setPropertyString func(h uintptr, name, value string) int32
	setProperty       func(h uintptr, name string, format int32, data unsafe.Pointer) int32
	getPropertyString func(h uintptr, name string) *byte
	getProperty       func(h uintptr, name string, format int32, data unsafe.Pointer) int32
	observeProperty   func(h uintptr, userdata uint64, name string, format int32) int32
	unobserveProperty func(h uintptr, userdata uint64) int32
	waitEvent         func(h uintptr, timeout float64) *cEvent
	wakeup            func(h uintptr)
	hookAdd           func(h uintptr, userdata uint64, name string, priority int32) int32
	hookContinue      func(h uintptr, id uint64) int32
	requestLog        func(h uintptr, level string) int32
	errorString       func(code int32) *byte
	free              func(p unsafe.Pointer)
	clientName        func(h uintptr) *byte
}

var (
	libOnce sync.Once
	lib     *Library
	libErr  error
)

// Candidates lists where the shared library is looked for, in order. The
// first entries are PlayAnything's own install locations so a bundled copy
// always wins over whatever else is on the system.
func Candidates() []string {
	var out []string
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	if env := os.Getenv("PLAYANYTHING_LIBMPV"); env != "" {
		out = append(out, env)
	}
	switch runtime.GOOS {
	case "windows":
		out = append(out,
			filepath.Join(exeDir, "libmpv-2.dll"),
			filepath.Join(exeDir, "lib", "libmpv-2.dll"),
			filepath.Join(exeDir, "..", "mpv-bin", "libmpv-2.dll"),
			"libmpv-2.dll", // PATH / system search
		)
	case "darwin":
		out = append(out,
			filepath.Join(exeDir, "libmpv.2.dylib"),
			filepath.Join(exeDir, "..", "Frameworks", "libmpv.2.dylib"),
			"/opt/homebrew/lib/libmpv.2.dylib", "/usr/local/lib/libmpv.2.dylib", "/opt/local/lib/libmpv.2.dylib",
			"libmpv.2.dylib",
		)
	default:
		out = append(out,
			filepath.Join(exeDir, "libmpv.so.2"),
			"libmpv.so.2", "libmpv.so.1", "libmpv.so",
		)
	}
	return out
}

// Load locates and loads libmpv once per process.
func Load() (*Library, error) {
	libOnce.Do(func() { lib, libErr = load() })
	return lib, libErr
}

func load() (*Library, error) {
	var lastErr error
	for _, c := range Candidates() {
		h, err := purego.Dlopen(c, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			lastErr = err
			continue
		}
		l := &Library{Path: c}
		purego.RegisterLibFunc(&l.clientAPIVersion, h, "mpv_client_api_version")
		purego.RegisterLibFunc(&l.create, h, "mpv_create")
		purego.RegisterLibFunc(&l.initialize, h, "mpv_initialize")
		purego.RegisterLibFunc(&l.terminateDestroy, h, "mpv_terminate_destroy")
		purego.RegisterLibFunc(&l.setOptionString, h, "mpv_set_option_string")
		purego.RegisterLibFunc(&l.commandString, h, "mpv_command_string")
		purego.RegisterLibFunc(&l.setPropertyString, h, "mpv_set_property_string")
		purego.RegisterLibFunc(&l.setProperty, h, "mpv_set_property")
		purego.RegisterLibFunc(&l.getPropertyString, h, "mpv_get_property_string")
		purego.RegisterLibFunc(&l.getProperty, h, "mpv_get_property")
		purego.RegisterLibFunc(&l.observeProperty, h, "mpv_observe_property")
		purego.RegisterLibFunc(&l.unobserveProperty, h, "mpv_unobserve_property")
		purego.RegisterLibFunc(&l.waitEvent, h, "mpv_wait_event")
		purego.RegisterLibFunc(&l.wakeup, h, "mpv_wakeup")
		purego.RegisterLibFunc(&l.hookAdd, h, "mpv_hook_add")
		purego.RegisterLibFunc(&l.hookContinue, h, "mpv_hook_continue")
		purego.RegisterLibFunc(&l.requestLog, h, "mpv_request_log_messages")
		purego.RegisterLibFunc(&l.errorString, h, "mpv_error_string")
		purego.RegisterLibFunc(&l.free, h, "mpv_free")
		purego.RegisterLibFunc(&l.clientName, h, "mpv_client_name")
		l.Version = l.clientAPIVersion()
		if l.Version>>16 < 1 {
			return nil, fmt.Errorf("%s: client API %d.%d is too old", c, l.Version>>16, l.Version&0xFFFF)
		}
		return l, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no candidate paths")
	}
	return nil, fmt.Errorf("playback engine (libmpv) not found: %w", lastErr)
}

// VersionString formats the client API version (e.g. "2.2").
func (l *Library) VersionString() string {
	return fmt.Sprintf("%d.%d", l.Version>>16, l.Version&0xFFFF)
}

// Engine is one libmpv core instance.
type Engine struct {
	lib     *Library
	h       uintptr
	events  chan Event
	stop    chan struct{}
	done    chan struct{}
	once    sync.Once
	mu      sync.Mutex
	closed  bool
	started bool // pump running (Init succeeded)
}

// New creates an uninitialised core. Set options, then call Init.
func New() (*Engine, error) {
	l, err := Load()
	if err != nil {
		return nil, err
	}
	h := l.create()
	if h == 0 {
		return nil, errors.New("mpv_create failed")
	}
	return &Engine{lib: l, h: h, events: make(chan Event, 256), stop: make(chan struct{}), done: make(chan struct{})}, nil
}

// Library returns the loaded library (path, version).
func (e *Engine) Library() *Library { return e.lib }

func (e *Engine) errf(code int32, what string) error {
	if code >= 0 {
		return nil
	}
	return fmt.Errorf("%s: %s", what, cstr(e.lib.errorString(code)))
}

// SetOption sets an option before Init (after Init, use SetProperty).
func (e *Engine) SetOption(name, value string) error {
	return e.errf(e.lib.setOptionString(e.h, name, value), "option "+name)
}

// Init initialises the core and starts the event pump.
func (e *Engine) Init() error {
	if err := e.errf(e.lib.initialize(e.h), "mpv_initialize"); err != nil {
		return err
	}
	e.mu.Lock()
	e.started = true
	e.mu.Unlock()
	go e.pump()
	return nil
}

// Events delivers decoded events. The channel is closed after Close.
func (e *Engine) Events() <-chan Event { return e.events }

// RequestLog enables log messages at the given level ("warn", "info", "v", "no").
func (e *Engine) RequestLog(level string) error {
	return e.errf(e.lib.requestLog(e.h, level), "request log")
}

// Command runs a command, e.g. Command("loadfile", path, "replace").
// Arguments are quoted for mpv's command parser.
func (e *Engine) Command(args ...string) error {
	if len(args) == 0 {
		return errors.New("empty command")
	}
	var sb strings.Builder
	for i, a := range args {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(quote(a))
	}
	return e.errf(e.lib.commandString(e.h, sb.String()), "command "+args[0])
}

// quote renders an argument for mpv's command-string parser.
func quote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\"'\\#;\n") && !strings.HasPrefix(s, "!") {
		return s
	}
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\n':
			sb.WriteString(`\n`)
		default:
			sb.WriteRune(r)
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

// SetProperty sets a property from a Go value (string, bool, int, int64, float64).
func (e *Engine) SetProperty(name string, value any) error {
	switch v := value.(type) {
	case string:
		return e.errf(e.lib.setPropertyString(e.h, name, v), "set "+name)
	case bool:
		var f int32
		if v {
			f = 1
		}
		return e.errf(e.lib.setProperty(e.h, name, int32(FormatFlag), unsafe.Pointer(&f)), "set "+name)
	case int:
		i := int64(v)
		return e.errf(e.lib.setProperty(e.h, name, int32(FormatInt64), unsafe.Pointer(&i)), "set "+name)
	case int64:
		i := v
		return e.errf(e.lib.setProperty(e.h, name, int32(FormatInt64), unsafe.Pointer(&i)), "set "+name)
	case float64:
		d := v
		return e.errf(e.lib.setProperty(e.h, name, int32(FormatDouble), unsafe.Pointer(&d)), "set "+name)
	}
	return fmt.Errorf("set %s: unsupported value type %T", name, value)
}

// GetString reads a property as a string ("" with an error when unavailable).
func (e *Engine) GetString(name string) (string, error) {
	p := e.lib.getPropertyString(e.h, name)
	if p == nil {
		return "", fmt.Errorf("property %s unavailable", name)
	}
	s := cstr(p)
	e.lib.free(unsafe.Pointer(p))
	return s, nil
}

// GetBool reads a flag property.
func (e *Engine) GetBool(name string) (bool, error) {
	var f int32
	if err := e.errf(e.lib.getProperty(e.h, name, int32(FormatFlag), unsafe.Pointer(&f)), "get "+name); err != nil {
		return false, err
	}
	return f != 0, nil
}

// GetInt reads an integer property.
func (e *Engine) GetInt(name string) (int64, error) {
	var i int64
	if err := e.errf(e.lib.getProperty(e.h, name, int32(FormatInt64), unsafe.Pointer(&i)), "get "+name); err != nil {
		return 0, err
	}
	return i, nil
}

// GetFloat reads a double property.
func (e *Engine) GetFloat(name string) (float64, error) {
	var d float64
	if err := e.errf(e.lib.getProperty(e.h, name, int32(FormatDouble), unsafe.Pointer(&d)), "get "+name); err != nil {
		return 0, err
	}
	return d, nil
}

// Observe subscribes to property changes; events carry ReplyUserdata=userdata.
func (e *Engine) Observe(userdata uint64, name string, format Format) error {
	return e.errf(e.lib.observeProperty(e.h, userdata, name, int32(format)), "observe "+name)
}

// Unobserve removes subscriptions registered with userdata.
func (e *Engine) Unobserve(userdata uint64) error {
	return e.errf(e.lib.unobserveProperty(e.h, userdata), "unobserve")
}

// HookAdd registers for a hook such as "on_load"; the event must be answered
// with HookContinue.
func (e *Engine) HookAdd(userdata uint64, name string, priority int) error {
	return e.errf(e.lib.hookAdd(e.h, userdata, name, int32(priority)), "hook "+name)
}

// HookContinue lets mpv proceed after a hook event.
func (e *Engine) HookContinue(id uint64) error {
	return e.errf(e.lib.hookContinue(e.h, id), "hook continue")
}

// Close shuts the core down and waits for the pump to exit.
func (e *Engine) Close() {
	e.once.Do(func() {
		e.mu.Lock()
		e.closed = true
		started := e.started
		e.mu.Unlock()
		close(e.stop)
		if started {
			e.lib.wakeup(e.h)
			<-e.done
		}
		e.lib.terminateDestroy(e.h)
		close(e.events)
	})
}

// pump polls mpv_wait_event and forwards decoded events. A short timeout
// keeps the OS thread free most of the time without callbacks into Go from
// foreign threads.
func (e *Engine) pump() {
	defer close(e.done)
	for {
		select {
		case <-e.stop:
			return
		default:
		}
		ev := e.lib.waitEvent(e.h, 0.05)
		if ev == nil || ev.eventID == int32(EventNone) {
			continue
		}
		out := decode(ev)
		if out.ID == EventShutdown {
			select {
			case e.events <- out:
			default:
			}
			return
		}
		select {
		case e.events <- out:
		case <-time.After(2 * time.Second):
			// Consumer stalled; drop rather than wedge the core.
		case <-e.stop:
			return
		}
	}
}

func decode(ev *cEvent) Event {
	out := Event{ID: EventID(ev.eventID), Error: ev.err, ReplyUserdata: ev.replyUserdata}
	switch out.ID {
	case EventPropertyChange:
		p := (*cEventProperty)(ev.data)
		out.Name = cstr(p.name)
		out.Format = Format(p.format)
		switch out.Format {
		case FormatString:
			if p.data != nil {
				out.Str = cstr(*(**byte)(p.data))
			}
		case FormatFlag:
			if p.data != nil {
				out.Flag = *(*int32)(p.data) != 0
			}
		case FormatInt64:
			if p.data != nil {
				out.Int = *(*int64)(p.data)
			}
		case FormatDouble:
			if p.data != nil {
				out.Double = *(*float64)(p.data)
			}
		}
	case EventHook:
		h := (*cEventHook)(ev.data)
		out.Name = cstr(h.name)
		out.HookID = h.id
	case EventEndFile:
		f := (*cEventEndFile)(ev.data)
		out.EndReason = f.reason
		out.EndError = f.err
		out.PlaylistEntryID = f.playlistEntryID
	case EventLogMessage:
		m := (*cEventLogMessage)(ev.data)
		out.LogPrefix, out.LogLevel, out.LogText = cstr(m.prefix), cstr(m.level), strings.TrimRight(cstr(m.text), "\n")
	case EventClientMessage:
		m := (*cEventClientMessage)(ev.data)
		if m.numArgs > 0 && m.args != nil {
			args := unsafe.Slice(m.args, int(m.numArgs))
			for _, a := range args {
				out.Args = append(out.Args, cstr(a))
			}
		}
	}
	return out
}

// ErrorText returns the message for an end-file error code.
func (e *Engine) ErrorText(code int32) string { return cstr(e.lib.errorString(code)) }

func cstr(p *byte) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}
