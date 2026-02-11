package server

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"phira-mp/common"
)

// ReplayRecorder 回放录制器
type ReplayRecorder struct {
	mu sync.RWMutex

	// 房间录制记录 roomId -> *RoomRecorder
	roomRecorders map[string]*RoomRecorder

	httpServer *HTTPServer
}

// RoomRecorder 房间录制器
type RoomRecorder struct {
	RoomID   string
	ChartID  int32
	File     *os.File
	FilePath string
	mu       sync.Mutex
}

// NewReplayRecorder 创建回放录制器
func NewReplayRecorder(httpServer *HTTPServer) *ReplayRecorder {
	r := &ReplayRecorder{
		roomRecorders: make(map[string]*RoomRecorder),
		httpServer:    httpServer,
	}

	// 启动清理协程
	go r.cleanupLoop()

	return r
}

// StartRecording 开始录制房间
func (r *ReplayRecorder) StartRecording(room *Room) error {
	if r.httpServer == nil || !r.httpServer.IsReplayEnabled() {
		return nil
	}

	chart := room.GetChart()
	if chart == nil {
		return fmt.Errorf("no chart selected")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// 检查是否已经在录制
	if _, ok := r.roomRecorders[room.ID.Value]; ok {
		return nil
	}

	// 为每个用户创建录制文件
	for _, user := range room.GetUsers() {
		recorder, err := r.createRecorder(room.ID.Value, chart.ID, user.ID)
		if err != nil {
			log.Printf("创建回放录制文件失败: %v", err)
			continue
		}
		r.roomRecorders[fmt.Sprintf("%s_%d", room.ID.Value, user.ID)] = recorder
	}

	log.Printf("开始录制房间 %s 的回放", room.ID.Value)
	return nil
}

// createRecorder 创建录制器
func (r *ReplayRecorder) createRecorder(roomID string, chartID, userID int32) (*RoomRecorder, error) {
	timestamp := time.Now().UnixMilli()

	// 创建目录
	dir := filepath.Join("record", fmt.Sprintf("%d", userID), fmt.Sprintf("%d", chartID))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	// 创建文件
	filePath := filepath.Join(dir, fmt.Sprintf("%d.phirarec", timestamp))
	file, err := os.Create(filePath)
	if err != nil {
		return nil, err
	}

	// 写入文件头
	// 2字节: 文件标识 (0x504D)
	// 4字节: 谱面ID
	// 4字节: 用户ID
	// 4字节: 成绩ID (初始为0)
	header := make([]byte, 14)
	binary.LittleEndian.PutUint16(header[0:2], 0x504D)
	binary.LittleEndian.PutUint32(header[2:6], uint32(chartID))
	binary.LittleEndian.PutUint32(header[6:10], uint32(userID))
	binary.LittleEndian.PutUint32(header[10:14], 0) // 成绩ID，游戏结束后再更新

	if _, err := file.Write(header); err != nil {
		file.Close()
		os.Remove(filePath)
		return nil, err
	}

	return &RoomRecorder{
		RoomID:   roomID,
		ChartID:  chartID,
		File:     file,
		FilePath: filePath,
	}, nil
}

// RecordTouch 录制触摸数据
func (r *ReplayRecorder) RecordTouch(roomID string, userID int32, frames []common.TouchFrame) {
	r.mu.RLock()
	recorder, ok := r.roomRecorders[fmt.Sprintf("%s_%d", roomID, userID)]
	r.mu.RUnlock()

	if !ok || recorder == nil {
		return
	}

	recorder.mu.Lock()
	defer recorder.mu.Unlock()

	// 写入触摸数据
	// 格式: [命令类型(1字节)] [数据长度(变长)] [数据]
	// 命令类型: 0x01 = TouchFrame
	for _, frame := range frames {
		// 写入命令类型
		if _, err := recorder.File.Write([]byte{0x01}); err != nil {
			log.Printf("写入回放数据失败: %v", err)
			return
		}

		// 序列化帧数据
		data := r.serializeTouchFrame(frame)

		// 写入长度
		lengthBytes := make([]byte, binary.MaxVarintLen64)
		n := binary.PutUvarint(lengthBytes, uint64(len(data)))
		if _, err := recorder.File.Write(lengthBytes[:n]); err != nil {
			log.Printf("写入回放数据失败: %v", err)
			return
		}

		// 写入数据
		if _, err := recorder.File.Write(data); err != nil {
			log.Printf("写入回放数据失败: %v", err)
			return
		}
	}
}

// RecordJudge 录制判定数据
func (r *ReplayRecorder) RecordJudge(roomID string, userID int32, judges []common.JudgeEvent) {
	r.mu.RLock()
	recorder, ok := r.roomRecorders[fmt.Sprintf("%s_%d", roomID, userID)]
	r.mu.RUnlock()

	if !ok || recorder == nil {
		return
	}

	recorder.mu.Lock()
	defer recorder.mu.Unlock()

	// 写入判定数据
	// 命令类型: 0x02 = JudgeEvent
	for _, judge := range judges {
		// 写入命令类型
		if _, err := recorder.File.Write([]byte{0x02}); err != nil {
			log.Printf("写入回放数据失败: %v", err)
			return
		}

		// 序列化判定数据
		data := r.serializeJudgeEvent(judge)

		// 写入长度
		lengthBytes := make([]byte, binary.MaxVarintLen64)
		n := binary.PutUvarint(lengthBytes, uint64(len(data)))
		if _, err := recorder.File.Write(lengthBytes[:n]); err != nil {
			log.Printf("写入回放数据失败: %v", err)
			return
		}

		// 写入数据
		if _, err := recorder.File.Write(data); err != nil {
			log.Printf("写入回放数据失败: %v", err)
			return
		}
	}
}

// StopRecording 停止录制房间
func (r *ReplayRecorder) StopRecording(roomID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 找到并关闭该房间的所有录制器
	for key, recorder := range r.roomRecorders {
		if recorder.RoomID == roomID {
			recorder.mu.Lock()
			recorder.File.Close()
			recorder.mu.Unlock()
			delete(r.roomRecorders, key)
		}
	}

	log.Printf("停止录制房间 %s 的回放", roomID)
}

// UpdateRecordID 更新录制文件的成绩ID
func (r *ReplayRecorder) UpdateRecordID(roomID string, userID, recordID int32) {
	r.mu.RLock()
	recorder, ok := r.roomRecorders[fmt.Sprintf("%s_%d", roomID, userID)]
	r.mu.RUnlock()

	if !ok || recorder == nil {
		return
	}

	recorder.mu.Lock()
	defer recorder.mu.Unlock()

	// 更新文件头中的成绩ID (偏移10字节)
	recordIDBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(recordIDBytes, uint32(recordID))

	recorder.File.Seek(10, 0)
	recorder.File.Write(recordIDBytes)
	recorder.File.Seek(0, 2) // 回到文件末尾
}

// serializeTouchFrame 序列化触摸帧
func (r *ReplayRecorder) serializeTouchFrame(frame common.TouchFrame) []byte {
	// 计算所需空间
	size := 4 + 1 // Time(float32) + Points count
	for range frame.Points {
		size += 1 + 4 // ID(int8) + Pos(CompactPos = 2*uint16 = 4 bytes)
	}

	data := make([]byte, size)
	offset := 0

	// Time
	binary.LittleEndian.PutUint32(data[offset:], uint32(frame.Time))
	offset += 4

	// Points count
	data[offset] = byte(len(frame.Points))
	offset++

	// Points
	for _, p := range frame.Points {
		data[offset] = byte(p.ID)
		offset++
		binary.LittleEndian.PutUint16(data[offset:], p.Pos.X)
		offset += 2
		binary.LittleEndian.PutUint16(data[offset:], p.Pos.Y)
		offset += 2
	}

	return data
}

// serializeJudgeEvent 序列化判定事件
func (r *ReplayRecorder) serializeJudgeEvent(judge common.JudgeEvent) []byte {
	data := make([]byte, 13)

	// Time (float32)
	binary.LittleEndian.PutUint32(data[0:], uint32(judge.Time))

	// LineID (uint32)
	binary.LittleEndian.PutUint32(data[4:], judge.LineID)

	// NoteID (uint32)
	binary.LittleEndian.PutUint32(data[8:], judge.NoteID)

	// Judgement (uint8)
	data[12] = byte(judge.Judgement)

	return data
}

// cleanupLoop 清理循环 - 删除超过4天的回放文件
func (r *ReplayRecorder) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	// 立即执行一次清理
	r.cleanupOldReplays()

	for range ticker.C {
		r.cleanupOldReplays()
	}
}

// cleanupOldReplays 清理旧回放文件
func (r *ReplayRecorder) cleanupOldReplays() {
	recordDir := "record"
	
	// 检查目录是否存在
	if _, err := os.Stat(recordDir); os.IsNotExist(err) {
		return
	}

	// 遍历所有用户目录
	userDirs, err := os.ReadDir(recordDir)
	if err != nil {
		log.Printf("清理旧回放文件失败: %v", err)
		return
	}

	cutoffTime := time.Now().Add(-4 * 24 * time.Hour) // 4天前

	for _, userDir := range userDirs {
		if !userDir.IsDir() {
			continue
		}

		userPath := filepath.Join(recordDir, userDir.Name())
		chartDirs, err := os.ReadDir(userPath)
		if err != nil {
			continue
		}

		for _, chartDir := range chartDirs {
			if !chartDir.IsDir() {
				continue
			}

			chartPath := filepath.Join(userPath, chartDir.Name())
			files, err := os.ReadDir(chartPath)
			if err != nil {
				continue
			}

			for _, file := range files {
				if file.IsDir() {
					continue
				}

				// 解析文件名获取时间戳
				fileName := file.Name()
				if len(fileName) < 14 || !strings.HasSuffix(fileName, ".phirarec") {
					continue
				}

				timestampStr := strings.TrimSuffix(fileName, ".phirarec")
				timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
				if err != nil {
					continue
				}

				fileTime := time.UnixMilli(timestamp)
				if fileTime.Before(cutoffTime) {
					// 删除旧文件
					filePath := filepath.Join(chartPath, fileName)
					if err := os.Remove(filePath); err != nil {
						log.Printf("删除旧回放文件失败 %s: %v", filePath, err)
					} else {
						log.Printf("删除旧回放文件: %s", filePath)
					}
				}
			}
		}
	}
}

// StopAllRecordings 停止所有录制
func (r *ReplayRecorder) StopAllRecordings() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for key, recorder := range r.roomRecorders {
		recorder.mu.Lock()
		recorder.File.Close()
		recorder.mu.Unlock()
		delete(r.roomRecorders, key)
	}

	log.Printf("停止所有回放录制")
}
