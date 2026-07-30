//go:build windows

package platform

import (
	"errors"
	"io"
	"io/fs"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"ansm/internal/logrelay"
	"ansm/internal/params"
	"ansm/internal/redirect"
	"ansm/internal/rotate"
)

const (
	genericRead    = 0x80000000
	genericWrite   = 0x40000000
	fileAppendData = 0x00000004

	openExisting = 3
	openAlways   = 4

	handleFlagInherit   = 0x00000001
	duplicateSameAccess = 0x00000002

	errorBrokenPipe = 109
	errorHandleEOF  = 38
)

var (
	procCreateFileW          = kernel32.NewProc("CreateFileW")
	procCreatePipe           = kernel32.NewProc("CreatePipe")
	procReadFile             = kernel32.NewProc("ReadFile")
	procWriteFile            = kernel32.NewProc("WriteFile")
	procSetHandleInformation = kernel32.NewProc("SetHandleInformation")
	procDuplicateHandle      = kernel32.NewProc("DuplicateHandle")
	procFlushFileBuffers     = kernel32.NewProc("FlushFileBuffers")
)

type securityAttributes struct {
	Length             uint32
	SecurityDescriptor uintptr
	InheritHandle      uint32
}

func inheritableAttributes(inherit bool) securityAttributes {
	sa := securityAttributes{Length: uint32(unsafe.Sizeof(securityAttributes{}))}
	if inherit {
		sa.InheritHandle = 1
	}
	return sa
}

// writeAccess picks the access mask for a redirected output file.
//
// Appending dispositions get FILE_APPEND_DATA without FILE_WRITE_DATA so every
// write lands at the end of the file no matter who else is writing. That is
// what makes a shared stdout/stderr file and copy-and-truncate rotation behave:
// the write position is never stale.
func writeAccess(disposition uint32) uint32 {
	switch disposition {
	case openExisting, openAlways:
		return fileAppendData | synchronize
	default:
		return genericWrite
	}
}

func createFile(path string, access uint32, stream redirect.Stream, inherit bool) (uintptr, error) {
	name, err := ptr(path)
	if err != nil {
		return 0, err
	}
	sa := inheritableAttributes(inherit)
	handle, _, callErr := procCreateFileW.Call(
		uintptr(unsafe.Pointer(name)),
		uintptr(access),
		uintptr(stream.ShareMode),
		uintptr(unsafe.Pointer(&sa)),
		uintptr(stream.CreationDisposition),
		uintptr(stream.FlagsAndAttributes),
		0,
	)
	if handle == ^uintptr(0) || handle == 0 {
		return 0, callErr
	}
	return handle, nil
}

func createPipe() (read, write uintptr, err error) {
	sa := inheritableAttributes(true)
	ret, _, callErr := procCreatePipe.Call(
		uintptr(unsafe.Pointer(&read)),
		uintptr(unsafe.Pointer(&write)),
		uintptr(unsafe.Pointer(&sa)),
		0,
	)
	if err = lastCallError(ret, callErr); err != nil {
		return 0, 0, err
	}
	// The parent keeps the read end; the child must not inherit it or the pipe
	// would never break and the relay would never see the end of the output.
	ret, _, callErr = procSetHandleInformation.Call(read, handleFlagInherit, 0)
	if err = lastCallError(ret, callErr); err != nil {
		procCloseHandle.Call(read)
		procCloseHandle.Call(write)
		return 0, 0, err
	}
	return read, write, nil
}

func closeHandle(handle uintptr) error {
	if handle == 0 {
		return nil
	}
	ret, _, callErr := procCloseHandle.Call(handle)
	return lastCallError(ret, callErr)
}

func duplicateInheritable(handle uintptr) (uintptr, error) {
	current, _, _ := procGetCurrentProcess.Call()
	var duplicate uintptr
	ret, _, callErr := procDuplicateHandle.Call(
		current,
		handle,
		current,
		uintptr(unsafe.Pointer(&duplicate)),
		0,
		1,
		duplicateSameAccess,
	)
	if err := lastCallError(ret, callErr); err != nil {
		return 0, err
	}
	return duplicate, nil
}

// pipeReader reads one relayed stream. A broken pipe is the normal end of a
// child's output, so it reads as io.EOF rather than an error.
type pipeReader struct {
	mu     sync.Mutex
	handle uintptr
}

func (p *pipeReader) Read(buffer []byte) (int, error) {
	p.mu.Lock()
	handle := p.handle
	p.mu.Unlock()
	if handle == 0 {
		return 0, fs.ErrClosed
	}
	if len(buffer) == 0 {
		return 0, nil
	}
	var read uint32
	ret, _, callErr := procReadFile.Call(
		handle,
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(len(buffer)),
		uintptr(unsafe.Pointer(&read)),
		0,
	)
	if err := lastCallError(ret, callErr); err != nil {
		if errnoIs(err, syscall.Errno(errorBrokenPipe)) || errnoIs(err, syscall.Errno(errorHandleEOF)) {
			return int(read), io.EOF
		}
		if errnoIs(err, syscall.Errno(errorInvalidHandle)) {
			return int(read), fs.ErrClosed
		}
		return int(read), err
	}
	if read == 0 {
		return 0, io.EOF
	}
	return int(read), nil
}

func (p *pipeReader) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	handle := p.handle
	p.handle = 0
	return closeHandle(handle)
}

// fileSink is one log file. Two relays share a sink when stdout and stderr name
// the same file, which is why every operation takes the lock.
type fileSink struct {
	mu     sync.Mutex
	stream redirect.Stream
	config redirect.Config
	flag   *logrelay.Flag
	handle uintptr
}

func newFileSink(stream redirect.Stream, config redirect.Config) (*fileSink, error) {
	sink := &fileSink{stream: stream, config: config, flag: logrelay.NewFlag(config.Online())}
	if err := sink.open(); err != nil {
		return nil, err
	}
	return sink, nil
}

func (s *fileSink) open() error {
	handle, err := createFile(s.stream.Path, writeAccess(s.stream.CreationDisposition), s.stream, false)
	if err != nil {
		return err
	}
	s.handle = handle
	return nil
}

func (s *fileSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle == 0 {
		return 0, fs.ErrClosed
	}
	if len(p) == 0 {
		return 0, nil
	}
	var written uint32
	ret, _, callErr := procWriteFile.Call(
		s.handle,
		uintptr(unsafe.Pointer(&p[0])),
		uintptr(len(p)),
		uintptr(unsafe.Pointer(&written)),
		0,
	)
	if err := lastCallError(ret, callErr); err != nil {
		return int(written), err
	}
	return int(written), nil
}

// Rotate swaps the file underneath an already running child. The handle is
// closed first so a rename can succeed, and reopened right after so the relay
// can carry on with the next line.
func (s *fileSink) Rotate(at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle == 0 {
		return fs.ErrClosed
	}
	procFlushFileBuffers.Call(s.handle)
	closeErr := closeHandle(s.handle)
	s.handle = 0
	// An on-demand rotation is not subject to the age and size criteria: the
	// operator asked for it (L0008 2.14).
	options := rotate.Options{CopyAndTruncate: s.stream.CopyAndTruncate, Delay: s.config.RotateDelay}
	_, rotateErr := rotate.Apply(s.stream.Path, rotate.Criteria{}, options, at)
	return errors.Join(closeErr, rotateErr, s.open())
}

func (s *fileSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	handle := s.handle
	s.handle = 0
	return closeHandle(handle)
}

// relayTask ties one pipe to the sink its contents end up in.
type relayTask struct {
	source *pipeReader
	relay  *logrelay.Relay
	sink   *fileSink
}

type windowsRedirection struct {
	config redirect.Config
	child  [3]Handle
	// hookMaster holds non-inheritable stdout/stderr sources. Each hook gets
	// short-lived inheritable duplicates, matching NSSM's use_output_handles().
	hookMaster [2]uintptr
	// pending holds the parent's copies of the child-side handles. They are
	// closed by Begin so that the child alone owns them.
	pending []uintptr
	tasks   []*relayTask
	sinks   []*fileSink
	group   sync.WaitGroup
	started bool
	closed  sync.Once
}

func (r *windowsRedirection) Handles() (Handle, Handle, Handle) {
	return r.child[0], r.child[1], r.child[2]
}

func (r *windowsRedirection) Begin() {
	if r.started {
		return
	}
	r.started = true
	r.releasePending()
	for _, task := range r.tasks {
		task := task
		r.group.Add(1)
		go func() {
			defer r.group.Done()
			_ = task.relay.Run(task.source, task.sink)
		}()
	}
}

func (r *windowsRedirection) Rotate() {
	for _, sink := range r.sinks {
		sink.flag.Request()
	}
}

func (r *windowsRedirection) OpenHookOutput() (Handle, Handle, func(), error) {
	var opened [2]uintptr
	cleanup := func() {
		for i, handle := range opened {
			closeHandle(handle)
			opened[i] = 0
		}
	}
	for i, master := range r.hookMaster {
		if master == 0 {
			continue
		}
		duplicate, err := duplicateInheritable(master)
		if err != nil {
			cleanup()
			return 0, 0, func() {}, err
		}
		opened[i] = duplicate
	}
	return Handle(opened[0]), Handle(opened[1]), cleanup, nil
}

func (r *windowsRedirection) Close() error {
	var err error
	r.closed.Do(func() {
		r.releasePending()
		r.releaseHookMasters()
		// The child is gone by now, so every write end of every pipe is closed
		// and the relays are about to see the end of their input. Give them the
		// cleanup window to drain what is still buffered, then close the read
		// handles and let a relay which is somehow still blocked give up.
		r.waitForRelays(params.LoggerCleanupDeadline)
		for _, task := range r.tasks {
			err = errors.Join(err, task.source.Close())
		}
		r.waitForRelays(params.LoggerCleanupDeadline)
		for _, sink := range r.sinks {
			err = errors.Join(err, sink.Close())
		}
	})
	return err
}

func (r *windowsRedirection) waitForRelays(deadline time.Duration) bool {
	done := make(chan struct{})
	go func() {
		r.group.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(deadline):
		return false
	}
}

func (r *windowsRedirection) releasePending() {
	for _, handle := range r.pending {
		closeHandle(handle)
	}
	r.pending = nil
}

func (r *windowsRedirection) releaseHookMasters() {
	seen := make(map[uintptr]bool)
	for i, handle := range r.hookMaster {
		if handle != 0 && !seen[handle] {
			closeHandle(handle)
			seen[handle] = true
		}
		r.hookMaster[i] = 0
	}
}

// rotateAtStartup applies the age and size criteria before the file is opened.
//
// A failure here is not fatal: the original logs and carries on, because losing
// the rotation is far better than refusing to start the service over it. The
// usual cause is another process still holding the file, which is exactly what
// AppStd*CopyAndTruncate exists for.
func rotateAtStartup(config redirect.Config, stream redirect.Stream, at time.Time) {
	if !config.RotateFiles || !stream.Enabled() {
		return
	}
	options := rotate.Options{CopyAndTruncate: stream.CopyAndTruncate, Delay: config.RotateDelay}
	_, _ = rotate.Apply(stream.Path, config.Criteria(), options, at)
}

func (r *windowsRedirection) openOutput(stream redirect.Stream, at time.Time) (Handle, *fileSink, uintptr, error) {
	rotateAtStartup(r.config, stream, at)
	if !r.config.Relayed(stream) {
		// Direct connection: the child writes to the file itself and the parent
		// never sees a byte of it.
		master, err := createFile(stream.Path, writeAccess(stream.CreationDisposition), stream, false)
		if err != nil {
			return 0, nil, 0, err
		}
		child, err := duplicateInheritable(master)
		if err != nil {
			closeHandle(master)
			return 0, nil, 0, err
		}
		r.pending = append(r.pending, child)
		return Handle(child), nil, master, nil
	}
	sink, err := newFileSink(stream, r.config)
	if err != nil {
		return 0, nil, 0, err
	}
	r.sinks = append(r.sinks, sink)
	read, write, err := createPipe()
	if err != nil {
		sink.Close()
		return 0, nil, 0, err
	}
	ret, _, callErr := procSetHandleInformation.Call(write, handleFlagInherit, 0)
	if err = lastCallError(ret, callErr); err != nil {
		closeHandle(read)
		closeHandle(write)
		sink.Close()
		return 0, nil, 0, err
	}
	child, err := duplicateInheritable(write)
	if err != nil {
		closeHandle(read)
		closeHandle(write)
		sink.Close()
		return 0, nil, 0, err
	}
	r.pending = append(r.pending, child)
	r.attach(read, sink)
	return Handle(child), sink, write, nil
}

func (r *windowsRedirection) attach(read uintptr, sink *fileSink) {
	r.tasks = append(r.tasks, &relayTask{
		source: &pipeReader{handle: read},
		relay: &logrelay.Relay{
			Timestamp: r.config.Timestamp,
			Rotate:    sink.flag,
		},
		sink: sink,
	})
}

// OpenRedirect opens every configured stream, rotating the output files first.
//
// When stdout and stderr name the same file they share a single destination:
// a shared handle for the direct path, a shared sink for the relayed path.
// Two independent handles would keep two write positions into one file and the
// second one would overwrite what the first had just written.
func (Windows) OpenRedirect(config redirect.Config) (Redirection, error) {
	r := &windowsRedirection{config: config}
	at := time.Now()

	if config.Stdin.Enabled() {
		handle, err := createFile(config.Stdin.Path, genericRead, config.Stdin, true)
		if err != nil {
			r.Close()
			return nil, err
		}
		r.pending = append(r.pending, handle)
		r.child[0] = Handle(handle)
	}

	var stdoutSink *fileSink
	if config.Stdout.Enabled() {
		handle, sink, master, err := r.openOutput(config.Stdout, at)
		if err != nil {
			r.Close()
			return nil, err
		}
		r.child[1] = handle
		r.hookMaster[0] = master
		stdoutSink = sink
	}

	if config.Stderr.Enabled() {
		switch {
		case config.SameTarget() && stdoutSink != nil:
			// Relayed and shared: a second pipe into the same sink, so each
			// stream keeps its own line state but one file is written.
			read, write, err := createPipe()
			if err != nil {
				r.Close()
				return nil, err
			}
			ret, _, callErr := procSetHandleInformation.Call(write, handleFlagInherit, 0)
			if err = lastCallError(ret, callErr); err != nil {
				closeHandle(read)
				closeHandle(write)
				r.Close()
				return nil, err
			}
			child, err := duplicateInheritable(write)
			if err != nil {
				closeHandle(read)
				closeHandle(write)
				r.Close()
				return nil, err
			}
			r.pending = append(r.pending, child)
			r.attach(read, stdoutSink)
			r.child[2] = Handle(child)
			r.hookMaster[1] = write
		case config.SameTarget() && r.child[1] != 0:
			// Direct and shared: the child gets the same handle twice. The
			// stderr share mode and disposition are not consulted, exactly as
			// in the original, because the file is already open.
			r.child[2] = r.child[1]
			r.hookMaster[1] = r.hookMaster[0]
		default:
			handle, _, master, err := r.openOutput(config.Stderr, at)
			if err != nil {
				r.Close()
				return nil, err
			}
			r.child[2] = handle
			r.hookMaster[1] = master
		}
	}
	return r, nil
}
