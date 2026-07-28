package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/mp3"
	"github.com/gopxl/beep/v2/speaker"
	"github.com/gopxl/beep/v2/wav"

	"github.com/pyq0109/mirgo/internal/log"
)

const (
	speakerSampleRate = 44100
	speakerBufferSize = speakerSampleRate / 4 // 250ms
	maxPolyphony      = 32
)

var gSound *SoundEngine

type cachedWAV struct {
	buf    *beep.Buffer
	format beep.Format
}

type SoundEngine struct {
	sfxMixer   *beep.Mixer
	musicMixer *beep.Mixer

	soundList []string
	cache     map[string]*cachedWAV
	cacheMu   sync.Mutex

	sfxEnabled atomic.Bool
	bgmEnabled atomic.Bool

	dataDir    string
	activeSfx  atomic.Int32
}

func NewSoundEngine(dataDir string) (*SoundEngine, error) {
	if err := speaker.Init(speakerSampleRate, speakerBufferSize); err != nil {
		return nil, err
	}

	se := &SoundEngine{
		sfxMixer:   &beep.Mixer{},
		musicMixer: &beep.Mixer{},
		cache:      make(map[string]*cachedWAV),
		dataDir:    dataDir,
	}
	se.sfxMixer.KeepAlive(true)
	se.musicMixer.KeepAlive(true)
	se.sfxEnabled.Store(true)
	se.bgmEnabled.Store(true)

	speaker.Play(se.sfxMixer)
	speaker.Play(se.musicMixer)

	lstPath := filepath.Join(dataDir, "wav", "sound.lst")
	se.soundList = loadSoundList(lstPath)
	log.Logf(log.LevelInfo, "Sound", "loaded %d sound entries from %s", len(se.soundList), lstPath)

	return se, nil
}

func (se *SoundEngine) PlaySound(idx int) {
	if se == nil || !se.sfxEnabled.Load() {
		return
	}
	if idx < 0 || idx >= len(se.soundList) || se.soundList[idx] == "" {
		return
	}
	path := se.resolvePath(se.soundList[idx])
	if _, err := os.Stat(path); err != nil {
		return
	}
	if se.activeSfx.Load() >= maxPolyphony {
		return
	}

	buf, format, err := se.getCachedWAV(path)
	if err != nil {
		return
	}

	streamer := buf.Streamer(0, buf.Len())
	if format.SampleRate != speakerSampleRate {
		streamer2 := beep.Resample(4, format.SampleRate, speakerSampleRate, streamer)
		se.activeSfx.Add(1)
		done := beep.Callback(func() { se.activeSfx.Add(-1) })
		speaker.Lock()
		se.sfxMixer.Add(beep.Seq(streamer2, done))
		speaker.Unlock()
		return
	}

	se.activeSfx.Add(1)
	done := beep.Callback(func() { se.activeSfx.Add(-1) })
	speaker.Lock()
	se.sfxMixer.Add(beep.Seq(streamer, done))
	speaker.Unlock()
}

func (se *SoundEngine) PlayBGM(relPath string) {
	if se == nil || !se.bgmEnabled.Load() {
		return
	}
	if relPath == "" {
		return
	}
	path := se.resolvePath(relPath)
	f, err := os.Open(path)
	if err != nil {
		return
	}

	streamer, format, err := wav.Decode(f)
	if err != nil {
		f.Close()
		return
	}

	// 解码到统一采样率的 Buffer，Buffer.Streamer 返回 StreamSeeker 可供 Loop 使用
	targetFormat := beep.Format{SampleRate: speakerSampleRate, NumChannels: 2, Precision: 2}
	buf := beep.NewBuffer(targetFormat)
	if format.SampleRate != speakerSampleRate {
		buf.Append(beep.Resample(4, format.SampleRate, speakerSampleRate, streamer))
	} else {
		buf.Append(streamer)
	}
	streamer.Close()

	if buf.Len() == 0 {
		return
	}

	// SilenceSound 停掉一切现有音效和 BGM（Delphi SoundUtil:258）
	se.SilenceSound()

	loop := beep.Loop(-1, buf.Streamer(0, buf.Len()))
	speaker.Lock()
	se.sfxMixer.Add(loop)
	speaker.Unlock()
}

func (se *SoundEngine) PlayMapMusic(musicID int) {
	if se == nil {
		return
	}
	if musicID < 0 || !se.bgmEnabled.Load() {
		se.StopMapMusic()
		return
	}
	relPath := filepath.Join("Music", strconv.Itoa(musicID)+".mp3")
	path := se.resolvePath(relPath)
	if _, err := os.Stat(path); err != nil {
		return
	}

	// 先停旧音乐（Delphi SoundUtil:279-280）
	se.StopMapMusic()

	f, err := os.Open(path)
	if err != nil {
		return
	}
	streamer, format, err := mp3.Decode(f)
	if err != nil {
		f.Close()
		return
	}

	var s beep.Streamer = streamer
	if format.SampleRate != speakerSampleRate {
		s = beep.Resample(4, format.SampleRate, speakerSampleRate, streamer)
	}

	cleanup := beep.Callback(func() { streamer.Close() })
	speaker.Lock()
	se.musicMixer.Add(beep.Seq(s, cleanup))
	speaker.Unlock()
	log.Logf(log.LevelInfo, "Sound", "playing map music: %s", relPath)
}

func (se *SoundEngine) StopMapMusic() {
	if se == nil {
		return
	}
	speaker.Lock()
	se.musicMixer.Clear()
	speaker.Unlock()
}

func (se *SoundEngine) SilenceSound() {
	if se == nil {
		return
	}
	speaker.Lock()
	se.sfxMixer.Clear()
	speaker.Unlock()
	se.activeSfx.Store(0)
}

func (se *SoundEngine) ToggleSFX() bool {
	if se == nil {
		return false
	}
	on := !se.sfxEnabled.Load()
	se.sfxEnabled.Store(on)
	if !on {
		se.SilenceSound()
	}
	return on
}

func (se *SoundEngine) ToggleBGM() bool {
	if se == nil {
		return false
	}
	on := !se.bgmEnabled.Load()
	se.bgmEnabled.Store(on)
	if !on {
		se.SilenceSound()
		se.StopMapMusic()
	}
	return on
}

func (se *SoundEngine) SFXEnabled() bool {
	if se == nil {
		return false
	}
	return se.sfxEnabled.Load()
}

func (se *SoundEngine) BGMEnabled() bool {
	if se == nil {
		return false
	}
	return se.bgmEnabled.Load()
}

func (se *SoundEngine) Close() {
	if se == nil {
		return
	}
	se.SilenceSound()
	se.StopMapMusic()
	speaker.Close()
}

func (se *SoundEngine) resolvePath(rel string) string {
	rel = strings.ReplaceAll(rel, "\\", string(filepath.Separator))
	return filepath.Join(se.dataDir, rel)
}

func (se *SoundEngine) getCachedWAV(path string) (*beep.Buffer, beep.Format, error) {
	se.cacheMu.Lock()
	if c, ok := se.cache[path]; ok {
		se.cacheMu.Unlock()
		return c.buf, c.format, nil
	}
	se.cacheMu.Unlock()

	f, err := os.Open(path)
	if err != nil {
		return nil, beep.Format{}, err
	}
	streamer, format, err := wav.Decode(f)
	if err != nil {
		f.Close()
		return nil, beep.Format{}, err
	}

	buf := beep.NewBuffer(format)
	buf.Append(streamer)
	streamer.Close()

	se.cacheMu.Lock()
	se.cache[path] = &cachedWAV{buf: buf, format: format}
	se.cacheMu.Unlock()

	return buf, format, nil
}

// loadSoundList 解析 sound.lst（Delphi SoundUtil:151-178）。
// 格式：索引: 相对路径，; 注释行，索引严格递增，稀疏缺口用空串占位。
func loadSoundList(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var list []string
	maxIdx := -1
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		idxStr, filePart, found := splitSoundLine(line)
		if !found {
			continue
		}
		n, err := strconv.Atoi(idxStr)
		if err != nil {
			n = 0
		}
		if n <= maxIdx {
			continue
		}
		for len(list) <= n {
			list = append(list, "")
		}
		list[n] = strings.TrimSpace(filePart)
		maxIdx = n
	}
	return list
}

func splitSoundLine(line string) (idx, file string, ok bool) {
	for i, ch := range line {
		if ch == ':' || ch == ' ' || ch == '\t' {
			idx = strings.TrimSpace(line[:i])
			file = strings.TrimSpace(line[i+1:])
			if idx != "" && file != "" {
				return idx, file, true
			}
			// 冒号后面可能有空格，继续找下一个分隔符
			if ch == ':' {
				file = strings.TrimSpace(line[i+1:])
				if file != "" {
					return idx, file, true
				}
			}
			return "", "", false
		}
	}
	return "", "", false
}
