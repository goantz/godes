// Copyright 2015 Alex Goussiatiner. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.
//
// Godes  is the general-purpose simulation library
// which includes the  simulation engine  and building blocks
// for modeling a wide variety of systems at varying levels of details.
//
// Godes model controls the runnners
// See examples for the usage.
//

package godes

import (
	"container/list"
	"fmt"

	//"sync"
	"time"
)

// simulationSecondScale 仿真时钟步进：100毫秒；
const simulationSecondScale = 100

// ===============================================================================
// 以下状态取值用于标记Runner的各种状态；

// rUNNER_STATE_READY 准备状态
const rUNNER_STATE_READY = 0

// rUNNER_STATE_ACTIVE 活动状态
const rUNNER_STATE_ACTIVE = 1

// rUNNER_STATE_WAITTING_COND 等待条件状态
const rUNNER_STATE_WAITING_COND = 2

// rUNNER_STATE_SCHEDULED 排程状态；
const rUNNER_STATE_SCHEDULED = 3

// rUNNER_STATE_INTERRUPTED 中断状态
const rUNNER_STATE_INTERRUPTED = 4

// rUNNER_STATE_TERMINATED 终止状态
const rUNNER_STATE_TERMINATED = 5

//=================================================================================

var modl *model

// 当前仿真时刻
var stime float64 = 0

// WaitUntilDone stops the main goroutine and waits
// until all the runners finished executing the Run()
// 停止模型的主协程运行，并等待所有Runner执行完成Run()函数；
func WaitUntilDone() {
	if modl == nil {
		panic(" not initilized")
	}
	modl.waitUntillDone()
}

//AddRunner adds the runner obejct into model
// 增加一个Runner（需实现了RunnerInterface接口）到模型中（激活Runner）；
func AddRunner(runner RunnerInterface) {
	if runner == nil {
		panic("runner is nil")
	}
	if modl == nil {
		createModel(false)
	}
	modl.add(runner)
}

//Interrupt holds the runner execution
// 挂起指定的Runner的执行；
func Interrupt(runner RunnerInterface) {
	if runner == nil {
		panic("runner is nil")
	}
	if modl == nil {
		panic("model is nil")
	}
	modl.interrupt(runner)
}

//Resume restarts the runner execution
// 恢复指定Runner的执行；
func Resume(runner RunnerInterface, timeChange float64) {
	if runner == nil {
		panic("runner is nil")
	}
	if modl == nil {
		panic("model is nil")
	}
	modl.resume(runner, timeChange)
}

//Run starts the simulation model.
// Must be called explicitly.
// 启动模型；
func Run() {
	// 如果模型不存在，则创建模型
	if modl == nil {
		createModel(false)
	}
	//assuming that it comes from the main go routine
	if modl.activeRunner == nil {
		panic("runner is nil")
	}
	if modl.activeRunner.getInternalId() != 0 {
		panic("it comes from not from the main go routine")
	}
	modl.simulationActive = true
	modl.control()
}

//Advance the simulation time 推进仿真时间，按指定的时间间隔推进模型的仿真时钟；
func Advance(interval float64) {
	// 如果模型不存在，则生成模型，否则让模型根据时间间隔进行仿真时钟的推进；
	if modl == nil {
		createModel(false)
	}
	modl.advance(interval)
}

// Verbose sets the model in the verbose mode
// 设置模型为debug模式；
func Verbose(v bool) {
	if modl == nil {
		createModel(v)
	}
	modl.DEBUG = v
}

// Clear the model between the runs
// 清除模型（重置模型）；
func Clear() {
	if modl == nil {
		panic(" No model exist")
	} else {

		stime = 0
		modl = newModel(modl.DEBUG)
		//model.simulationActive = true
		//model.control()
	}
}

// GetSystemTime retuns the current simulation time
// 返回当前仿真时刻
func GetSystemTime() float64 {
	return stime
}

// Yield stops the runner for short time
// 停止Runner极短的一个时间
func Yield() {
	Advance(0.01)
}

// createModel
// 创建模型
func createModel(verbose bool) {
	if modl != nil {
		panic("model is already active")
	}
	stime = 0
	modl = newModel(verbose)
	//model.simulationActive = true
	//model.control()
	//assuming that it comes from the main go routine
}

// model 模型，是整个仿真的运行核心，负责根据仿真时钟，进行各种Runner的推进工作；
type model struct {
	//mu                  sync.RWMutex

	// 当前活动的Runner
	activeRunner RunnerInterface

	// 当前推进中的Runner列表
	movingList *list.List

	// 当前排程中的Runner列表
	scheduledList *list.List

	// 当前等待中的runner列表；
	waitingList *list.List

	// 等待条件中的Runner列表；
	waitingConditionMap map[int]RunnerInterface

	// 中断中的Runner列表；
	interruptedMap map[int]RunnerInterface

	// 终止的Runner列表；
	terminatedList *list.List

	// 当前ID
	currentId int

	// 控制用channel
	controlChannel chan int

	// 模型运行标记
	simulationActive bool

	// debug标记；
	DEBUG bool
}

//newModel initilizes the model
// 初始化模型
func newModel(verbose bool) *model {

	// ball 初始化模型时的内置活动Runner，优先级设置为100（最低），只为了保持模型的运行（活动）
	// 没有实际性的计算内容存在；

	var ball *Runner = newRunner()
	ball.channel = make(chan int)
	ball.markTime = time.Now()
	ball.internalId = 0
	ball.state = rUNNER_STATE_ACTIVE //that is bypassing READY
	ball.priority = 100
	ball.setMarkTime(time.Now())
	var runner RunnerInterface = ball
	mdl := model{activeRunner: runner, controlChannel: make(chan int), DEBUG: verbose, simulationActive: false}
	mdl.addToMovingList(runner)
	return &mdl
}

// advance 按指定的时间间隔推进仿真时钟，推进成功返回true；
func (mdl *model) advance(interval float64) bool {

	// 获取模型中活动runner的通道；
	ch := mdl.activeRunner.getChannel()

	// 推进活动Runner的时间为当前仿真时刻+指定的时间间隔；
	mdl.activeRunner.setMovingTime(stime + interval)
	// 设置活动Runner的状态为排程状态；
	mdl.activeRunner.setState(rUNNER_STATE_SCHEDULED)

	// 将活动Runner从推动列表中移出。
	mdl.removeFromMovingList(mdl.activeRunner)

	// 将活动Runner加入排程列表；
	mdl.addToSchedulledList(mdl.activeRunner)

	//restart control channel and freez
	// 重启控制channel 并 freez ？？TODO
	mdl.controlChannel <- 100

	// 等待活动Runner的channel（信号？执行完成？）
	<-ch

	return true
}

func (mdl *model) waitUntillDone() {

	// 如果模型的当前活动Runner不是Ball，则Panic，为什么 TODO
	if mdl.activeRunner.getInternalId() != 0 {
		panic("waitUntillDone initiated for not main ball")
	}

	// 从推进列表中移除当前活动Runner
	mdl.removeFromMovingList(mdl.activeRunner)

	// 为什么是100 塞入 channel
	mdl.controlChannel <- 100

	// 启动循环，判断模型是否不在仿真活动状态，如果不在仿真活动状态，则直接退出运行；
	// 如果在仿真活动状态，并且处于Debug模式下，则打印推进列表的长度；
	// 同时，将主协程休眠100毫秒；
	for {
		if !modl.simulationActive {
			break
		} else {
			if mdl.DEBUG {
				fmt.Println("waiting", mdl.movingList.Len())
			}
			time.Sleep(time.Millisecond * simulationSecondScale)
		}
	}
}

// add 增加Runner
func (mdl *model) add(runner RunnerInterface) bool {

	// 把当前ID加一；
	mdl.currentId++

	runner.setChannel(make(chan int))

	// 设置runner的推进时间为默认仿真时间
	runner.setMovingTime(stime)
	// 设置runner的内部ID为当前ID；
	runner.setInternalId(mdl.currentId)
	// 设置runner的初始状态为Ready状态；
	runner.setState(rUNNER_STATE_READY)
	// 将runner进入推进中列表；
	mdl.addToMovingList(runner)

	//启动一个协程专门跑runner
	go func() {
		// 等待runner的触发信号；
		<-runner.getChannel()
		//在收到触发信息后，进行如下操作：

		// 设置runner的标记时间为当前时间；
		runner.setMarkTime(time.Now())
		// 执行runner的Run()函数；
		runner.Run()
		if mdl.activeRunner == nil {
			panic("remove: activeRunner == nil")
		}
		// 从推进列表中移除模型当前活动的Runner ？？ 为什么？ TODO
		mdl.removeFromMovingList(mdl.activeRunner)

		mdl.activeRunner.setState(rUNNER_STATE_TERMINATED)
		mdl.activeRunner = nil
		mdl.controlChannel <- 100
	}()
	return true

}

func (mdl *model) interrupt(runner RunnerInterface) {

	if runner.getState() != rUNNER_STATE_SCHEDULED {
		panic("It is not  rUNNER_STATE_SCHEDULED")
	}
	mdl.removeFromSchedulledList(runner)
	runner.setState(rUNNER_STATE_INTERRUPTED)
	mdl.addToInterruptedMap(runner)

}

func (mdl *model) resume(runner RunnerInterface, timeChange float64) {
	if runner.getState() != rUNNER_STATE_INTERRUPTED {
		panic("It is not  rUNNER_STATE_INTERRUPTED")
	}
	mdl.removeFromInterruptedMap(runner)
	runner.setState(rUNNER_STATE_SCHEDULED)
	runner.setMovingTime(runner.getMovingTime() + timeChange)
	//mdl.addToMovingList(runner)
	mdl.addToSchedulledList(runner)

}

func (mdl *model) booleanControlWait(b *BooleanControl, val bool) {

	ch := mdl.activeRunner.getChannel()
	if mdl.activeRunner == nil {
		panic("booleanControlWait - no runner")
	}

	mdl.removeFromMovingList(mdl.activeRunner)

	mdl.activeRunner.setState(rUNNER_STATE_WAITING_COND)
	mdl.activeRunner.setWaitingForBool(val)
	mdl.activeRunner.setWaitingForBoolControl(b)

	mdl.addToWaitingConditionMap(mdl.activeRunner)
	mdl.controlChannel <- 100
	<-ch

}

func (mdl *model) booleanControlWaitAndTimeout(b *BooleanControl, val bool, timeout float64) {

	ri := &TimeoutRunner{&Runner{}, mdl.activeRunner, timeout}
	AddRunner(ri)
	mdl.activeRunner.setWaitingForBoolControlTimeoutId(ri.getInternalId())
	mdl.booleanControlWait(b, val)

}

func (mdl *model) booleanControlSet(b *BooleanControl) {
	ch := mdl.activeRunner.getChannel()
	if mdl.activeRunner == nil {
		panic("booleanControlSet - no runner")
	}
	mdl.controlChannel <- 100
	<-ch

}

func (mdl *model) control() bool {

	if mdl.activeRunner == nil {
		panic("control: activeBall == nil")
	}

	go func() {
		var runner RunnerInterface
		for {
			<-mdl.controlChannel
			if mdl.waitingConditionMap != nil && len(mdl.waitingConditionMap) > 0 {
				for key, temp := range mdl.waitingConditionMap {
					if temp.getWaitingForBoolControl() == nil {
						panic("  no BoolControl")
					}
					if temp.getWaitingForBool() == temp.getWaitingForBoolControl().GetState() {
						temp.setState(rUNNER_STATE_READY)
						temp.setWaitingForBoolControl(nil)
						temp.setWaitingForBoolControlTimeoutId(-1)
						mdl.addToMovingList(temp)
						delete(mdl.waitingConditionMap, key)
						break
					}
				}
			}

			//finding new runner
			runner = nil
			if mdl.movingList != nil && mdl.movingList.Len() > 0 {
				runner = mdl.getFromMovingList()
			}
			if runner == nil && mdl.scheduledList != nil && mdl.scheduledList.Len() > 0 {
				runner = mdl.getFromSchedulledList()
				if runner.getMovingTime() < stime {
					panic("control is seting simulation time in the past")
				} else {
					stime = runner.getMovingTime()
				}
				mdl.addToMovingList(runner)
			}
			if runner == nil {
				break
			}
			//restarting
			mdl.activeRunner = runner
			mdl.activeRunner.setState(rUNNER_STATE_ACTIVE)
			runner.setWaitingForBoolControl(nil)
			mdl.activeRunner.getChannel() <- -1

		}
		if mdl.DEBUG {
			fmt.Println("Finished")
		}
		mdl.simulationActive = false
	}()

	return true

}

/*
MovingList
This list is sorted in descending order according to the value of the ball priority attribute.
Balls with identical priority values are sorted according to the FIFO principle.

			|
			|	|
			|	|
	     	|	|	|
	<-----	|	|	|	|
*/
func (mdl *model) addToMovingList(runner RunnerInterface) bool {

	if mdl.DEBUG {
		fmt.Printf("addToMovingList %v\n", runner)
	}

	if mdl.movingList == nil {
		mdl.movingList = list.New()
		mdl.movingList.PushFront(runner)
		return true
	}

	insertedSwt := false
	for element := mdl.movingList.Front(); element != nil; element = element.Next() {
		if runner.getPriority() > element.Value.(RunnerInterface).getPriority() {
			mdl.movingList.InsertBefore(runner, element)
			insertedSwt = true
			break
		}
	}
	if !insertedSwt {
		mdl.movingList.PushBack(runner)
	}
	return true
}

func (mdl *model) getFromMovingList() RunnerInterface {

	if mdl.movingList == nil {
		panic("MovingList was not initilized")
	}
	if mdl.DEBUG {
		runner := mdl.movingList.Front().Value.(RunnerInterface)
		fmt.Printf("getFromMovingList %v\n", runner)
	}
	runner := mdl.movingList.Front().Value.(RunnerInterface)
	mdl.movingList.Remove(mdl.movingList.Front())
	return runner

}

func (mdl *model) removeFromMovingList(runner RunnerInterface) {

	if mdl.movingList == nil {
		panic("MovingList was not initilized")
	}

	if mdl.DEBUG {
		fmt.Printf("removeFromMovingList %v\n", runner)
	}
	var found bool
	for e := mdl.movingList.Front(); e != nil; e = e.Next() {
		if e.Value == runner {
			mdl.movingList.Remove(e)
			found = true
			break
		}
	}

	if !found {
		//panic("not found in MovingList")
	}
}

/*
SchedulledList
This list is sorted in descending order according to the schedulled time
Priorites are not used

			|
			|	|
			|	|
			|	|	|
			|	|	|	|--->

*/
func (mdl *model) addToSchedulledList(runner RunnerInterface) bool {

	if mdl.scheduledList == nil {
		mdl.scheduledList = list.New()
		mdl.scheduledList.PushFront(runner)
		return true
	}
	insertedSwt := false
	for element := mdl.scheduledList.Back(); element != nil; element = element.Prev() {
		if runner.getMovingTime() < element.Value.(RunnerInterface).getMovingTime() {
			mdl.scheduledList.InsertAfter(runner, element)
			insertedSwt = true
			break
		}
	}
	if !insertedSwt {
		mdl.scheduledList.PushFront(runner)
	}

	if mdl.DEBUG {
		fmt.Println("===")
		fmt.Printf("addToSchedulledList %v\n", runner)
		for element := mdl.scheduledList.Front(); element != nil; element = element.Next() {
			fmt.Printf("elem %v\n", element.Value.(RunnerInterface))
		}
		fmt.Println("===")
	}
	return true
}

func (mdl *model) getFromSchedulledList() RunnerInterface {
	if mdl.scheduledList == nil {
		panic(" SchedulledList was not initilized")
	}
	if mdl.DEBUG {
		runner := mdl.scheduledList.Back().Value.(RunnerInterface)
		fmt.Printf("getFromSchedulledList %v\n", runner)
	}
	runner := mdl.scheduledList.Back().Value.(RunnerInterface)
	mdl.scheduledList.Remove(mdl.scheduledList.Back())
	return runner
}

func (mdl *model) removeFromSchedulledList(runner RunnerInterface) {
	if mdl.scheduledList == nil {
		panic("schedulledList was not initilized")
	}
	if modl.DEBUG {
		fmt.Printf("removeFrom schedulledListt %v\n", runner)
	}
	var found bool
	for e := mdl.scheduledList.Front(); e != nil; e = e.Next() {
		if e.Value == runner {
			mdl.scheduledList.Remove(e)
			found = true
			break
		}
	}
	if !found {
		panic("not found in scheduledList")
	}
	return
}

func (mdl *model) addToWaitingConditionMap(runner RunnerInterface) bool {

	if runner.getWaitingForBoolControl() == nil {
		panic(" addToWaitingConditionMap - no control ")
	}

	if mdl.DEBUG {
		fmt.Printf("addToWaitingConditionMap %v\n", runner)
	}

	if mdl.waitingConditionMap == nil {
		mdl.waitingConditionMap = make(map[int]RunnerInterface)

	}
	mdl.waitingConditionMap[runner.getInternalId()] = runner
	return true
}

func (mdl *model) addToInterruptedMap(runner RunnerInterface) bool {

	if mdl.DEBUG {
		fmt.Printf("addToInterruptedMap %v\n", runner)
	}
	if mdl.interruptedMap == nil {
		mdl.interruptedMap = make(map[int]RunnerInterface)
	}

	mdl.interruptedMap[runner.getInternalId()] = runner
	return true
}

func (mdl *model) removeFromInterruptedMap(runner RunnerInterface) bool {

	if mdl.DEBUG {
		fmt.Printf("removeFromInterruptedMap %v\n", runner)
	}

	_, ok := mdl.interruptedMap[runner.getInternalId()]

	if ok {
		delete(mdl.interruptedMap, runner.getInternalId())
	} else {
		panic("not found in interruptedMap")
	}
	return true
}
