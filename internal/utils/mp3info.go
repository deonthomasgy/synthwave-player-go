package utils

import (
	"encoding/binary"
	"io"
	"os"
	"strconv"
)

// mp3 bitrate tables in kbps, indexed by [versionGroup][layer][bitrateIndex].
// versionGroup: 0 = MPEG1, 1 = MPEG2/2.5
// layer index: 1 = Layer I, 2 = Layer II, 3 = Layer III
var mp3BitrateTable = map[int]map[int][]int{
	0: {
		1: {0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448},
		2: {0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384},
		3: {0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320},
	},
	1: {
		1: {0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256},
		2: {0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160},
		3: {0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160},
	},
}

var mp3SampleRate = map[int][]int{
	3: {44100, 48000, 32000}, // MPEG1
	2: {22050, 24000, 16000}, // MPEG2
	0: {11025, 12000, 8000},  // MPEG2.5
}

// MP3Bitrate returns the bitrate in kbps as a string (e.g. "320", or "192~" for
// VBR), reading the first audio frame. ok is false if the file is not a parseable
// MPEG audio file. This mirrors the Perl server's bitrate_kbps behavior.
func MP3Bitrate(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()

	start, err := skipID3v2(f)
	if err != nil {
		return "", false
	}

	hdr, frameOff, err := findFrameHeader(f, start)
	if err != nil {
		return "", false
	}

	v := versionFromHeader(hdr)
	layer := layerFromHeader(hdr)
	if layer == 0 {
		return "", false
	}
	vg := 0
	if v != 3 {
		vg = 1
	}
	brIndex := int((hdr >> 12) & 0x0F)
	srIndex := int((hdr >> 10) & 0x03)
	if brIndex == 0 || brIndex == 15 || srIndex == 3 {
		return "", false
	}
	table, ok := mp3BitrateTable[vg][layer]
	if !ok || brIndex >= len(table) {
		return "", false
	}
	frameBitrate := table[brIndex]
	sampleRate := mp3SampleRate[v][srIndex]
	if sampleRate == 0 {
		return "", false
	}

	if avg, vbr, ok := xingAverageBitrate(f, frameOff, hdr, v, layer, sampleRate); ok {
		s := strconv.Itoa(avg)
		if vbr {
			s += "~"
		}
		return s, true
	}

	return strconv.Itoa(frameBitrate), true
}

func skipID3v2(f *os.File) (int64, error) {
	head := make([]byte, 10)
	if _, err := f.ReadAt(head, 0); err != nil {
		return 0, err
	}
	if string(head[0:3]) == "ID3" {
		// Synchsafe 28-bit size in head[6..9].
		size := int64(head[6]&0x7f)<<21 | int64(head[7]&0x7f)<<14 | int64(head[8]&0x7f)<<7 | int64(head[9]&0x7f)
		return 10 + size, nil
	}
	return 0, nil
}

// findFrameHeader scans for the first valid MPEG frame sync and returns the
// 32-bit header plus the byte offset where the frame begins.
func findFrameHeader(f *os.File, start int64) (uint32, int64, error) {
	buf := make([]byte, 4096)
	off := start
	const maxScan = 1 << 20 // 1 MiB
	for off < start+maxScan {
		n, err := f.ReadAt(buf, off)
		if n < 4 {
			if err != nil {
				return 0, 0, err
			}
			return 0, 0, io.ErrUnexpectedEOF
		}
		for i := 0; i < n-3; i++ {
			if buf[i] == 0xFF && (buf[i+1]&0xE0) == 0xE0 {
				hdr := binary.BigEndian.Uint32(buf[i : i+4])
				if validFrameHeader(hdr) {
					return hdr, off + int64(i), nil
				}
			}
		}
		off += int64(n - 3)
		if err == io.EOF {
			break
		}
	}
	return 0, 0, io.ErrUnexpectedEOF
}

func validFrameHeader(hdr uint32) bool {
	if (hdr & 0xFFE00000) != 0xFFE00000 {
		return false
	}
	if versionFromHeaderRaw(hdr) == 1 { // reserved
		return false
	}
	if layerFromHeader(hdr) == 0 { // reserved
		return false
	}
	brIndex := (hdr >> 12) & 0x0F
	srIndex := (hdr >> 10) & 0x03
	if brIndex == 0 || brIndex == 15 || srIndex == 3 {
		return false
	}
	return true
}

func versionFromHeaderRaw(hdr uint32) int { return int((hdr >> 19) & 0x03) }

// versionFromHeader returns 3=MPEG1, 2=MPEG2, 0=MPEG2.5.
func versionFromHeader(hdr uint32) int { return int((hdr >> 19) & 0x03) }

// layerFromHeader returns 1=Layer I, 2=Layer II, 3=Layer III, 0=reserved.
func layerFromHeader(hdr uint32) int {
	switch (hdr >> 17) & 0x03 {
	case 3:
		return 1
	case 2:
		return 2
	case 1:
		return 3
	default:
		return 0
	}
}

// xingAverageBitrate reads a Xing/Info/VBRI header (if present) to compute the
// average bitrate over the whole stream. Returns (kbps, isVBR, ok).
func xingAverageBitrate(f *os.File, frameOff int64, hdr uint32, version, layer, sampleRate int) (int, bool, bool) {
	channelMode := (hdr >> 6) & 0x03
	mono := channelMode == 3

	var sideInfo int64
	if version == 3 { // MPEG1
		if mono {
			sideInfo = 17
		} else {
			sideInfo = 32
		}
	} else {
		if mono {
			sideInfo = 9
		} else {
			sideInfo = 17
		}
	}

	tag := make([]byte, 4)
	xingOff := frameOff + 4 + sideInfo
	vbr := false
	var foundTag string
	if _, err := f.ReadAt(tag, xingOff); err == nil {
		switch string(tag) {
		case "Xing":
			foundTag = "Xing"
			vbr = true
		case "Info":
			foundTag = "Info"
		}
	}
	// VBRI is always 32 bytes after the frame header.
	if foundTag == "" {
		if _, err := f.ReadAt(tag, frameOff+4+32); err == nil && string(tag) == "VBRI" {
			return vbriAverage(f, frameOff+4+32, samplesPerFrame(version, layer), sampleRate)
		}
		return 0, false, false
	}

	flags := make([]byte, 4)
	if _, err := f.ReadAt(flags, xingOff+4); err != nil {
		return 0, false, false
	}
	hasFrames := flags[3]&0x01 != 0
	hasBytes := flags[3]&0x02 != 0
	if !hasFrames || !hasBytes {
		return 0, false, false
	}
	cursor := xingOff + 8
	numFrames, err := readUint32(f, cursor)
	if err != nil {
		return 0, false, false
	}
	cursor += 4
	numBytes, err := readUint32(f, cursor)
	if err != nil {
		return 0, false, false
	}
	if numFrames == 0 || numBytes == 0 {
		return 0, false, false
	}
	spf := samplesPerFrame(version, layer)
	durationSec := float64(numFrames) * float64(spf) / float64(sampleRate)
	if durationSec <= 0 {
		return 0, false, false
	}
	avg := int(float64(numBytes) * 8 / durationSec / 1000)
	if avg <= 0 {
		return 0, false, false
	}
	return avg, vbr, true
}

func vbriAverage(f *os.File, vbriOff int64, spf, sampleRate int) (int, bool, bool) {
	// VBRI: bytes at offset+10 (4 bytes) = number of bytes, +14 (4 bytes) = number of frames.
	numBytes, err := readUint32(f, vbriOff+10)
	if err != nil {
		return 0, false, false
	}
	numFrames, err := readUint32(f, vbriOff+14)
	if err != nil || numFrames == 0 || numBytes == 0 {
		return 0, false, false
	}
	durationSec := float64(numFrames) * float64(spf) / float64(sampleRate)
	if durationSec <= 0 {
		return 0, false, false
	}
	avg := int(float64(numBytes) * 8 / durationSec / 1000)
	if avg <= 0 {
		return 0, false, false
	}
	return avg, true, true
}

func samplesPerFrame(version, layer int) int {
	switch layer {
	case 1:
		return 384
	case 2:
		return 1152
	default: // Layer III
		if version == 3 { // MPEG1
			return 1152
		}
		return 576
	}
}

func readUint32(f *os.File, off int64) (uint32, error) {
	b := make([]byte, 4)
	if _, err := f.ReadAt(b, off); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(b), nil
}
