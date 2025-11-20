package transmission

import (
	"sync"

	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
)

var (
	gTransSessionInitFlag bool
	gTransMutex           sync.Mutex
)

// TransServerInit 初始化传输服务器
// 对应C代码: core/transmission/session/src/trans_session_service.c:TransServerInit
func TransServerInit() error {
	gTransMutex.Lock()
	defer gTransMutex.Unlock()

	if gTransSessionInitFlag {
		return nil
	}

	// 初始化transmission auth manager
	if err := TransAuthInit(); err != nil {
		log.Errorf("[TRANS] TransAuthInit failed: %v", err)
		return err
	}

	gTransSessionInitFlag = true
	log.Info("[TRANS] Trans session server init success")
	return nil
}

// TransServerDeinit 反初始化传输服务器
// 对应C代码: core/transmission/session/src/trans_session_service.c:TransServerDeinit
func TransServerDeinit() {
	gTransMutex.Lock()
	defer gTransMutex.Unlock()

	if !gTransSessionInitFlag {
		return
	}

	TransAuthDeinit()
	gTransSessionInitFlag = false
	log.Info("[TRANS] Trans session server deinit")
}
