package wil

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// testImg 描述一张合成图片的尺寸（像素索引用调色板 1，统一颜色）。
type testImg struct {
	w, h int16
}

const (
	testR = 0xAA
	testG = 0xBB
	testB = 0xCC
)

// buildPalette 构造 256 色调色板（原始字节序为 [B,G,R,x]）。
// 条目 0 会被 Load 强制置为透明；条目 1..255 统一为不透明测试色。
func buildPalette() []byte {
	pal := make([]byte, 256*4)
	for i := 1; i < 256; i++ {
		off := i * 4
		pal[off+0] = testB
		pal[off+1] = testG
		pal[off+2] = testR
		pal[off+3] = 0x00 // A 由 Load 强制为 255（条目 0 除外置 0）
	}
	return pal
}

// buildWil 按指定 btVersion 合成 .wil 文件字节流，返回 (数据, 各图偏移)。
func buildWil(btVersion int, imgs []testImg) ([]byte, []int32) {
	var buf bytes.Buffer
	w := func(v any) { binary.Write(&buf, binary.LittleEndian, v) }

	buf.WriteString("WMAGC") // 5 字节 magic，非 "#ILIB"
	title := make([]byte, 36)
	copy(title, "test")
	buf.Write(title)
	w(int32(len(imgs))) // imgCount
	w(int32(256))       // colorCount <= 256 → 8bpp 调色板路径
	w(int32(0))         // paletteSize（未使用）

	pal := buildPalette()
	if btVersion == 0 {
		w(int32(1)) // verFlag 非 0 → btVersion 0
		buf.Write(pal)
	} else {
		w(int32(0)) // verFlag 0 → btVersion 1，Load 回退 4 字节，palette 从此处读起
		buf.Write(pal[4:])
	}

	offsets := make([]int32, len(imgs))
	for i, im := range imgs {
		offsets[i] = int32(buf.Len())
		w(im.w)
		w(im.h)
		w(int16(0)) // hotX
		w(int16(0)) // hotY
		if btVersion == 0 {
			w(uint32(0)) // bits 字段仅 btVersion 0 存在
		}
		rowBytes := widthBytes(int(im.w), 8)
		buf.Write(bytes.Repeat([]byte{1}, rowBytes*int(im.h))) // 全部像素 = 调色板索引 1
	}
	return buf.Bytes(), offsets
}

// buildWix 按指定 btVersion 合成 .wix 索引字节流。
func buildWix(btVersion int, offsets []int32) []byte {
	var buf bytes.Buffer
	w := func(v any) { binary.Write(&buf, binary.LittleEndian, v) }

	buf.WriteString("WIMAG") // 5 字节 magic，非 "#INDX"
	buf.Write(make([]byte, 36))
	w(int32(len(offsets))) // indexCount
	if btVersion == 0 {
		w(int32(1)) // verFlag 独立字段；btVersion 1 时该槽位即 offsets[0]
	}
	for _, off := range offsets {
		w(off)
	}
	return buf.Bytes()
}

// writeTestWil 在临时目录写入一对 .wil/.wix，返回 .wil 路径。
func writeTestWil(t *testing.T, btVersion int, imgs []testImg) string {
	t.Helper()
	dir := t.TempDir()
	data, offsets := buildWil(btVersion, imgs)
	wilPath := filepath.Join(dir, "test.wil")
	if err := os.WriteFile(wilPath, data, 0o644); err != nil {
		t.Fatalf("write wil: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "test.wix"), buildWix(btVersion, offsets), 0o644); err != nil {
		t.Fatalf("write wix: %v", err)
	}
	return wilPath
}

// resetCacheState 将全局缓存记账复位，避免测试间相互污染。
func resetCacheState(t *testing.T) {
	t.Helper()
	cacheMu.Lock()
	cacheLimit = DefaultCacheLimit
	cacheUsed = 0
	cacheMu.Unlock()
}

func usedBytes() int64 {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	return cacheUsed
}

func setLimit(t *testing.T, n int64) {
	t.Helper()
	SetCacheLimit(n)
	t.Cleanup(func() { SetCacheLimit(DefaultCacheLimit) })
}

// loadTest 加载一个合成文件并注册关闭清理。
func loadTest(t *testing.T, btVersion int, imgs []testImg) *File {
	t.Helper()
	f, err := Load(writeTestWil(t, btVersion, imgs))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	t.Cleanup(f.Close)
	return f
}

func TestLoadAndDecodePixels(t *testing.T) {
	resetCacheState(t)
	for _, bt := range []int{0, 1} {
		f := loadTest(t, bt, []testImg{{w: 5, h: 5}})
		img := f.GetImage(0)
		if img == nil || img.RGBA == nil {
			t.Fatalf("btVersion %d: GetImage returned nil image", bt)
		}
		if img.Width != 5 || img.Height != 5 {
			t.Fatalf("btVersion %d: dims = %dx%d, want 5x5", bt, img.Width, img.Height)
		}
		// 像素应为调色板 1 的颜色（R,G,B,A）
		pix := img.RGBA.Pix
		if pix[0] != testR || pix[1] != testG || pix[2] != testB || pix[3] != 0xFF {
			t.Fatalf("btVersion %d: pixel = %v, want [%#x %#x %#x ff]", bt, pix[:4], testR, testG, testB)
		}
		if got := int64(len(pix)); img.cachedBytes != got {
			t.Fatalf("btVersion %d: cachedBytes = %d, want %d", bt, img.cachedBytes, got)
		}
	}
}

func TestCacheHitSamePointer(t *testing.T) {
	resetCacheState(t)
	f := loadTest(t, 0, []testImg{{w: 4, h: 4}})
	first := f.GetImage(0)
	second := f.GetImage(0)
	if first != second {
		t.Fatal("cache miss: GetImage returned a different pointer on second call")
	}
	if got := usedBytes(); got != int64(len(first.RGBA.Pix)) {
		t.Fatalf("cacheUsed = %d, want %d (no double accounting)", got, len(first.RGBA.Pix))
	}
}

func TestLRUEvictionOrder(t *testing.T) {
	resetCacheState(t)
	// 三张 5x5（各 100 字节），预算容得下两张。
	setLimit(t, 250)
	f := loadTest(t, 0, []testImg{{5, 5}, {5, 5}, {5, 5}})

	f.GetImage(0) // LRU: [0]
	f.GetImage(1) // LRU: [1,0]
	f.GetImage(0) // 命中提升 → LRU: [0,1]
	f.GetImage(2) // 超额，应淘汰最久未用的 1，而非刚提升的 0

	if f.Images[1] != nil {
		t.Fatal("image 1 (least recently used) should have been evicted")
	}
	if f.Images[0] == nil || f.Images[2] == nil {
		t.Fatal("images 0 and 2 should remain cached")
	}
	if got := usedBytes(); got != 200 {
		t.Fatalf("cacheUsed = %d, want 200", got)
	}
}

func TestProtectJustInserted(t *testing.T) {
	resetCacheState(t)
	// 单图（256 字节）大于预算（100），刚插入的图不能被当轮淘汰。
	setLimit(t, 100)
	f := loadTest(t, 0, []testImg{{8, 8}})

	img := f.GetImage(0)
	if img == nil || img.RGBA == nil {
		t.Fatal("just-inserted oversized image must not be evicted in its own pass")
	}
	if f.Images[0] == nil {
		t.Fatal("image 0 should still be cached")
	}
}

func TestShellEntriesNotEvicted(t *testing.T) {
	resetCacheState(t)
	// 预算容一张半。图 0 释放成壳后，再加载图 1、2 触发淘汰：
	// 淘汰应跳过 0 字节壳（图 0），淘汰最久未用的有像素条目（图 1）。
	setLimit(t, 150)
	f := loadTest(t, 0, []testImg{{5, 5}, {5, 5}, {5, 5}})

	img0 := f.GetImage(0) // LRU: [0]
	f.ReleasePixels(0)    // 壳，cacheUsed=0
	f.GetImage(1)         // LRU: [1,0]，cacheUsed=100
	f.GetImage(2)         // cacheUsed=200>150 → 跳过壳 0，淘汰图 1

	if f.Images[0] != img0 {
		t.Fatal("shell entry 0 should be retained (metadata preserved)")
	}
	if img0.RGBA != nil {
		t.Fatal("shell entry 0 should have nil RGBA")
	}
	if img0.Width != 5 || img0.Height != 5 {
		t.Fatal("shell entry should preserve dimensions")
	}
	if f.Images[1] != nil {
		t.Fatal("image 1 (oldest with pixels) should have been evicted")
	}
	if f.Images[2] == nil || f.Images[2].RGBA == nil {
		t.Fatal("image 2 should be cached")
	}
}

func TestReleasePixels(t *testing.T) {
	resetCacheState(t)
	f := loadTest(t, 0, []testImg{{5, 5}})
	img := f.GetImage(0)
	size := int64(len(img.RGBA.Pix))
	if usedBytes() != size {
		t.Fatalf("cacheUsed = %d, want %d", usedBytes(), size)
	}

	f.ReleasePixels(0)
	if usedBytes() != 0 {
		t.Fatalf("after release, cacheUsed = %d, want 0", usedBytes())
	}
	if f.Images[0] == nil {
		t.Fatal("entry should remain as a shell")
	}
	if f.Images[0].RGBA != nil {
		t.Fatal("RGBA should be nil after release")
	}
	// 幂等
	f.ReleasePixels(0)
	if usedBytes() != 0 {
		t.Fatalf("double release changed cacheUsed to %d", usedBytes())
	}
	// 释放后 GetImage 命中壳，不重新解码
	if got := f.GetImage(0); got != img {
		t.Fatal("GetImage after release should return the same shell pointer")
	}
}

func TestUnlimitedMode(t *testing.T) {
	resetCacheState(t)
	setLimit(t, 0) // 不淘汰
	imgs := make([]testImg, 8)
	for i := range imgs {
		imgs[i] = testImg{8, 8} // 各 256 字节，共 2048
	}
	f := loadTest(t, 0, imgs)
	for i := range imgs {
		f.GetImage(i)
	}
	for i := range imgs {
		if f.Images[i] == nil || f.Images[i].RGBA == nil {
			t.Fatalf("image %d should be cached in unlimited mode", i)
		}
	}
	if got := usedBytes(); got != 8*256 {
		t.Fatalf("cacheUsed = %d, want %d", got, 8*256)
	}
}

func TestCloseRefundsBytes(t *testing.T) {
	resetCacheState(t)
	path := writeTestWil(t, 0, []testImg{{5, 5}, {5, 5}})
	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	f.GetImage(0)
	f.GetImage(1)
	if usedBytes() != 200 {
		t.Fatalf("cacheUsed = %d, want 200", usedBytes())
	}

	f.Close()
	if usedBytes() != 0 {
		t.Fatalf("after Close, cacheUsed = %d, want 0", usedBytes())
	}
	for i, img := range f.Images {
		if img != nil {
			t.Fatalf("image %d should be cleared after Close", i)
		}
	}
	// Close 幂等
	f.Close()
	if usedBytes() != 0 {
		t.Fatalf("double Close changed cacheUsed to %d", usedBytes())
	}
	// Close 后 GetImage 返回空哨兵且不影响记账
	if got := f.GetImage(0); got == nil || got.RGBA != nil {
		t.Fatal("GetImage after Close should return an empty sentinel")
	}
	if usedBytes() != 0 {
		t.Fatalf("sentinel after Close changed cacheUsed to %d", usedBytes())
	}
}

// 并发烟囱测试：确认记账锁路径无数据竞争（配合 -race 运行）。
func TestConcurrentGetImage(t *testing.T) {
	resetCacheState(t)
	f := loadTest(t, 0, []testImg{{4, 4}, {5, 5}, {6, 6}, {7, 7}})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				f.GetImage(j % 4)
			}
		}()
	}
	wg.Wait()
}
