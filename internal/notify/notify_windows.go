//go:build windows

package notify

import (
	"bytes"
	"encoding/binary"
	"math"
	"sync"
	"syscall"
	"unsafe"
)

var (
	winmm         = syscall.NewLazyDLL("winmm.dll")
	procPlaySound = winmm.NewProc("PlaySoundW")
)

const (
	sndAsync     = 0x0001
	sndMemory    = 0x0004
	sndNodefault = 0x0002
)

// A pleasant bell-like "ding": a decaying sine (with a soft octave partial),
// synthesized once into an in-memory WAV. This sounds far smoother than the
// square-wave kernel32.Beep and does not depend on the Windows sound scheme.
var (
	okWav  []byte
	errWav []byte
	once   sync.Once
)

func build() {
	okWav = makeDing(659.25, 0.60, 8.0)  // E5, light quick chime
	errWav = makeDing(261.63, 0.55, 8.5) // C4, low short tone for failures
}

// makeDing returns a 16-bit mono PCM WAV of a decaying bell tone.
func makeDing(freq, seconds, decay float64) []byte {
	const sampleRate = 44100
	n := int(sampleRate * seconds)
	var pcm bytes.Buffer
	for i := 0; i < n; i++ {
		t := float64(i) / sampleRate
		env := math.Exp(-t * decay)
		v := env * (0.85*math.Sin(2*math.Pi*freq*t) + 0.15*math.Sin(2*math.Pi*2*freq*t))
		s := int16(v * 11000)
		binary.Write(&pcm, binary.LittleEndian, s)
	}
	data := pcm.Bytes()
	var w bytes.Buffer
	w.WriteString("RIFF")
	binary.Write(&w, binary.LittleEndian, uint32(36+len(data)))
	w.WriteString("WAVE")
	w.WriteString("fmt ")
	binary.Write(&w, binary.LittleEndian, uint32(16)) // PCM chunk size
	binary.Write(&w, binary.LittleEndian, uint16(1))  // PCM
	binary.Write(&w, binary.LittleEndian, uint16(1))  // mono
	binary.Write(&w, binary.LittleEndian, uint32(sampleRate))
	binary.Write(&w, binary.LittleEndian, uint32(sampleRate*2)) // byte rate
	binary.Write(&w, binary.LittleEndian, uint16(2))            // block align
	binary.Write(&w, binary.LittleEndian, uint16(16))           // bits/sample
	w.WriteString("data")
	binary.Write(&w, binary.LittleEndian, uint32(len(data)))
	w.Write(data)
	return w.Bytes()
}

// Beep gives audible feedback. ok=true plays a soft chime, ok=false a low tone.
// Playback is asynchronous so it never blocks the hotkey loop.
func Beep(ok bool) {
	once.Do(build)
	wav := okWav
	if !ok {
		wav = errWav
	}
	procPlaySound.Call(
		uintptr(unsafe.Pointer(&wav[0])),
		0,
		uintptr(sndAsync|sndMemory|sndNodefault),
	)
}
