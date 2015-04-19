package post_finder

import (
	"fmt"
	"time"

	"github.com/purstal/pbtools/modules/postbar/accounts"
	"github.com/purstal/pbtools/modules/postbar/advsearch"
	"github.com/purstal/pbtools/modules/postbar/apis"
	"github.com/purstal/pbtools/modules/postbar/forum-win8-1.5.0.0"
	monitor "github.com/purstal/pbtools/tools-core/forum-page-monitor"
)

type Control int

const (
	Finish   Control = 0
	Continue Control = 1
)

type ThreadFilter func(account *accounts.Account, thread *ForumPageThread) (Control, string) //false则无需Continue,下同
type ThreadAssessor func(account *accounts.Account, thread *ForumPageThread) Control
type AdvSearchAssessor func(account *accounts.Account, result *advsearch.AdvSearchResult) Control
type PostAssessor func(account *accounts.Account, post *ThreadPagePost)
type CommentAssessor func(account *accounts.Account, comment *FloorPageComment)

type PostFinder struct {
	FreshPostMonitor        *monitor.FreshPostMonitor
	ServerTime              time.Time
	AccWin8, AccAndr        *accounts.Account
	ForumName               string
	Fid                     uint64
	ThreadFilter            ThreadFilter
	NewThreadFirstAssessor  ThreadAssessor
	NewThreadSecondAssessor PostAssessor
	AdvSearchAssessor       AdvSearchAssessor
	PostAssessor            PostAssessor
	CommentAssessor         CommentAssessor
	SearchTaskManager       *SearchTaskManager

	Debugger *Debugger
	Debug    struct {
		StartTime time.Time
	}
}

func init() {
	InitLoggers()
	InitDebugger()
}

func NewPostFinder(accWin8, accAndr *accounts.Account, forumName string, yield func(*PostFinder)) *PostFinder {
	var postFinder PostFinder
	postFinder.Debug.StartTime = time.Now()

	postFinder.AccWin8 = accWin8
	postFinder.AccAndr = accAndr
	postFinder.ForumName = forumName

	yield(&postFinder)
	if postFinder.ThreadFilter == nil || postFinder.NewThreadFirstAssessor == nil ||
		postFinder.NewThreadSecondAssessor == nil || postFinder.AdvSearchAssessor == nil ||
		postFinder.PostAssessor == nil || postFinder.CommentAssessor == nil {
		Logger.Fatal("删贴机初始化错误,有函数未设置:", postFinder, ".")
		panic("删贴机初始化错误,有函数未设置: " + fmt.Sprintln(postFinder) + ".")
	}

	fid, err, pberr := apis.GetFid(forumName)
	if err != nil || pberr != nil {
		Logger.Fatal("获取fid时出错: ", err, pberr)
		return nil
	}
	postFinder.Fid = fid

	postFinder.SearchTaskManager = NewSearchTaskManager(&postFinder, 0, time.Second,
		time.Second*10, time.Second*30, time.Minute, time.Minute*5, time.Minute*10,
		time.Minute*30, time.Hour, time.Hour*3)

	/*
		postFinder.SearchTaskManager = NewSearchTaskManager(&postFinder, 0, time.Second,
			time.Second*3, time.Second*3, time.Second*3)
	*/

	postFinder.Debugger = NewDebugger(forumName, &postFinder, time.Second/4)

	return &postFinder

}

type ForumPageThread struct {
	Forum  *forum.ForumPage
	Thread forum.ForumPageThread
	Extra  *forum.ForumPageExtra
}

func (finder *PostFinder) Run(monitorInterval time.Duration) {

	//monitor.MakeForumPageThreadChan(acc, "minecraft")
	var threadChan = make(chan ForumPageThread)
	finder.FreshPostMonitor = monitor.NewFreshPostMonitor(finder.AccWin8, finder.ForumName, monitorInterval)

	go func() {
		for {
			forumPage := <-finder.FreshPostMonitor.PageChan
			//fmt.Println(len(forumPage.ThreadList))
			if forumPage.Extra.ServerTime.After(finder.ServerTime) {
				finder.ServerTime = forumPage.Extra.ServerTime
			}
			//fmt.Println("---", forumPage.Extra.ServerTime)
			for _, thread := range forumPage.ThreadList {
				threadChan <- ForumPageThread{
					Forum:  forumPage.Forum,
					Thread: *thread,
					Extra:  forumPage.Extra,
				}
			}
		}
	}()

	go func() {
		for {
			thread := <-threadChan
			if finder.SearchTaskManager.Debug.CurrentServerTime.Before(thread.Extra.ServerTime) {
				finder.SearchTaskManager.Debug.CurrentServerTime = thread.Extra.ServerTime
			}
			if ctrl, _ := finder.ThreadFilter(finder.AccWin8, &thread); ctrl == Continue { //true:不忽略
				//第二项是原因
				if IsNewThread(&thread.Thread) {
					go finder.FindAndAnalyseNewThread(&thread)
				} else {
					go finder.FindAndAnalyseNewPost(&thread)

				}
			}
		}
	}()
}

func (finder *PostFinder) ChangeMonitorInterval(monitorInterval time.Duration) {
	finder.FreshPostMonitor.ChangeInterval(monitorInterval)
}
