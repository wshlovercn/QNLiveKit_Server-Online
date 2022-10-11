package impl

import (
	"context"
	"strings"

	"github.com/jinzhu/gorm"

	"github.com/qbox/livekit/biz/model"
	"github.com/qbox/livekit/core/rest"
	"github.com/qbox/livekit/module/biz/censor/config"
	"github.com/qbox/livekit/module/biz/censor/service"
	"github.com/qbox/livekit/module/store/mysql"
	"github.com/qbox/livekit/utils/logger"
)

type CensorServiceImpl struct {
	Callback string
	Bucket   string
	Addr     string
	Client   *CensorClient
}

var instance *CensorServiceImpl

func GetInstance() *CensorServiceImpl {
	return instance
}

func ConfigCensorService(conf *config.Config) {
	instance = &CensorServiceImpl{
		Callback: conf.Callback,
		Bucket:   conf.Bucket,
		Addr:     conf.Addr,
	}
}

func (c *CensorServiceImpl) PreStart() error {
	c.Client = NewCensorClient(c.Callback, c.Bucket)
	return nil
}

func (c *CensorServiceImpl) UpdateCensorConfig(ctx context.Context, mod *model.CensorConfig) error {
	log := logger.ReqLogger(ctx)
	db := mysql.GetLive(log.ReqID())
	mod.ID = 1
	err := db.Model(model.CensorConfig{}).Save(mod).Error
	if err != nil {
		return rest.ErrInternal
	}
	return nil
}

func (c *CensorServiceImpl) GetCensorConfig(ctx context.Context) (*model.CensorConfig, error) {
	log := logger.ReqLogger(ctx)
	db := mysql.GetLive(log.ReqID())
	mod := &model.CensorConfig{}
	result := db.Model(model.CensorConfig{}).First(mod)
	if result.Error != nil {
		if result.RecordNotFound() {
			mod.Enable = true
			mod.ID = 1
			mod.Pulp = true
			mod.Interval = 20
			err := db.Model(model.CensorConfig{}).Save(mod).Error
			if err != nil {
				return nil, rest.ErrInternal
			}
		} else {
			return nil, rest.ErrInternal
		}
	}
	return mod, nil
}

func (c *CensorServiceImpl) CreateCensorJob(ctx context.Context, liveEntity *model.LiveEntity) error {
	log := logger.ReqLogger(ctx)
	config, err := c.GetCensorConfig(ctx)
	if err != nil {
		log.Errorf("GetCensorConfig Error %v", err)
		return err
	}
	if config.Enable == false {
		return nil
	}
	resp, err := c.Client.JobCreate(ctx, liveEntity, config)
	if err != nil {
		log.Errorf("JobCreate Error %v", err)
		return err
	}
	err = c.SaveLiveCensorJob(ctx, liveEntity.LiveId, resp.Data.JobID, config)
	if err != nil {
		log.Errorf("SaveLiveCensorJob Error %v", err)
		return nil
	}
	return nil
}

func (c *CensorServiceImpl) SaveLiveCensorJob(ctx context.Context, liveId string, jobId string, config *model.CensorConfig) error {
	log := logger.ReqLogger(ctx)
	db := mysql.GetLive(log.ReqID())
	m := &model.LiveCensor{
		LiveID:     liveId,
		JobID:      jobId,
		Interval:   config.Interval,
		Politician: config.Politician,
		Pulp:       config.Pulp,
		Ads:        config.Ads,
		Terror:     config.Terror,
	}
	err := db.Model(model.LiveCensor{}).Save(m).Error
	if err != nil {
		return err
	}
	return nil
}

func (c *CensorServiceImpl) GetLiveCensorJobByLiveId(ctx context.Context, liveId string) (*model.LiveCensor, error) {
	log := logger.ReqLogger(ctx)
	db := mysql.GetLive(log.ReqID())
	m := &model.LiveCensor{}
	err := db.Model(model.LiveCensor{}).First(m, "live_id = ?", liveId).Error
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (c *CensorServiceImpl) StopCensorJob(ctx context.Context, liveId string) error {
	log := logger.ReqLogger(ctx)
	liveCensorJob, err := c.GetLiveCensorJobByLiveId(ctx, liveId)
	if err != nil {
		log.Errorf("GetLiveCensorJobByLiveId Error %v", err)
		return err
	}

	req := &JobCreateResponseData{
		JobID: liveCensorJob.JobID,
	}
	err = c.Client.JobClose(ctx, req)
	if err != nil {
		log.Errorf("post StopCensorJob Error %v", err)
		return err
	}
	return nil
}

func (c *CensorServiceImpl) GetCensorImageById(ctx context.Context, imageId uint) (*model.CensorImage, error) {
	log := logger.ReqLogger(ctx)
	db := mysql.GetLive(log.ReqID())
	m := &model.CensorImage{}
	err := db.Model(model.CensorImage{}).First(m, "id = ?", imageId).Error
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (c *CensorServiceImpl) GetLiveCensorJobByJobId(ctx context.Context, jobId string) (*model.LiveCensor, error) {
	log := logger.ReqLogger(ctx)
	db := mysql.GetLive(log.ReqID())
	m := &model.LiveCensor{}
	err := db.Model(model.LiveCensor{}).First(m, "job_id = ?", jobId).Error
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (c *CensorServiceImpl) SaveCensorImage(ctx context.Context, image *model.CensorImage) error {
	log := logger.ReqLogger(ctx)
	db := mysql.GetLive(log.ReqID())
	err := db.Model(model.CensorImage{}).Save(image).Error
	if err != nil {
		return err
	}
	return nil
}

func (c *CensorServiceImpl) GetUnreviewCount(ctx context.Context, liveId string) (len int, err error) {
	log := logger.ReqLogger(ctx)
	db := mysql.GetLiveReadOnly(log.ReqID())
	err = db.Model(&model.CensorImage{}).Where(" is_review = ? and live_id = ? ", 0, liveId).Count(&len).Error
	return
}

func (c *CensorServiceImpl) SearchCensorImage(ctx context.Context, isReview *int, pageNum, pageSize int, liveId *string) (image []model.CensorImage, totalCount int, err error) {
	log := logger.ReqLogger(ctx)
	db := mysql.GetLiveReadOnly(log.ReqID())
	image = make([]model.CensorImage, 0)

	var where *gorm.DB
	var all *gorm.DB
	if liveId == nil {
		where = db.Model(&model.CensorImage{}).Where(" is_review = ? ", isReview)
		all = db.Model(&model.CensorImage{})
	} else {
		where = db.Model(&model.CensorImage{}).Where(" is_review = ? and live_id = ?", isReview, liveId)
		all = db.Model(&model.CensorImage{}).Where(" live_id = ? ", liveId)
	}

	if isReview == nil {
		err = all.Order("created_at desc").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&image).Error
		err = all.Count(&totalCount).Error
	} else if *isReview == IsReviewNo {
		err = where.Order("created_at desc").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&image).Error
		err = where.Count(&totalCount).Error
	} else if *isReview == IsReviewYes {
		err = where.Count(&totalCount).Error
		err = where.Order("review_answer desc").Order("created_at desc ").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&image).Error
	}

	if err != nil {
		log.Errorf("search CensorImage error: %v", err)
		return
	}
	return
}

const IsReviewNo = 0
const IsReviewYes = 1

func (c *CensorServiceImpl) SearchCensorLive(ctx context.Context, isReview *int, pageNum, pageSize int) (censorLive []service.CensorLive, totalCount int, err error) {

	log := logger.ReqLogger(ctx)
	db := mysql.GetLiveReadOnly(log.ReqID())
	lives := make([]model.LiveEntity, 0)
	var db2 *gorm.DB
	if isReview == nil {
		db2 = db.Model(&model.LiveEntity{}).Where("unreview_censor_count >= 0")
	} else if *isReview == IsReviewNo {
		db2 = db.Model(&model.LiveEntity{}).Where("unreview_censor_count > 0").Where("stop_reason != ?", model.LiveStopReasonCensor)
	}
	err = db2.Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&lives).Error
	err = db2.Count(&totalCount).Error
	if err != nil {
		log.Errorf("SearchCensorLive %v", err)
		return nil, 0, err
	}

	for _, live := range lives {
		violationC := 0
		err = db.Model(&model.CensorImage{}).Where("live_id = ? and review_answer = ?", live.LiveId, model.AuditResultBlock).Count(&violationC).Error
		if err != nil {
			return nil, 0, err
		}
		aiC := 0
		err = db.Model(&model.CensorImage{}).Where("live_id = ? ", live.LiveId).Count(&aiC).Error
		if err != nil {
			return nil, 0, err
		}
		cl := service.CensorLive{
			LiveId:         live.LiveId,
			Title:          live.Title,
			AnchorId:       live.AnchorId,
			Status:         live.Status,
			Count:          live.UnreviewCensorCount,
			StopReason:     live.StopReason,
			ViolationCount: violationC,
			AiCount:        aiC,
			PushUrl:        live.PushUrl,
			RtmpPlayUrl:    live.RtmpPlayUrl,
			FlvPlayUrl:     live.FlvPlayUrl,
			HlsPlayUrl:     live.HlsPlayUrl,
		}
		if live.StopAt != nil {
			cl.StopAt = live.StopAt.UnixMilli()
		}
		if live.LastCensorTime != 0 {
			cl.Time = int64(live.LastCensorTime * 1000)
		}
		if live.StartAt != nil {
			cl.StartAt = live.StartAt.UnixMilli()
		}

		censorLive = append(censorLive, cl)
	}
	return
}

func (c *CensorServiceImpl) BatchUpdateCensorImage(ctx context.Context, images []uint, updates map[string]interface{}) error {
	log := logger.ReqLogger(ctx)
	db := mysql.GetLive(log.ReqID())
	db = db.Model(model.CensorImage{})

	result := db.Where(" id in (?) ", images).Update(updates)
	if result.Error != nil {
		log.Errorf("update user error %v", result.Error)
		return rest.ErrInternal
	} else {
		return nil
	}
}

type JobQueryRequest struct {
	Job         string   `json:"job"`
	Suggestions []string `json:"suggestions"`
	Start       int64    `json:"start"`
	End         int64    `json:"end"`
}

type JobQueryResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Marker string `json:"marker"`
		Items  struct {
			Image []struct {
				Code      int    `json:"code,omitempty"`
				Message   string `json:"message,omitempty"`
				Job       string `json:"job,omitempty"`
				Timestamp int    `json:"timestamp,omitempty"`
				Url       string `json:"url,omitempty"`
				Result    struct {
					Suggestion string `json:"suggestion"`
					Scenes     struct {
						Pulp struct {
							Suggestion string `json:"suggestion"`
							Details    []struct {
								Suggestion string  `json:"suggestion,omitempty"`
								Label      string  `json:"label,omitempty"`
								Score      float64 `json:"score,omitempty"`
							} `json:"details"`
						} `json:"pulp"`
					} `json:"scenes"`
				} `json:"result,omitempty"`
			} `json:"image"`
			Audio []struct {
				Code      int    `json:"code,omitempty"`
				Message   string `json:"message,omitempty"`
				Job       string `json:"job,omitempty"`
				Start     int    `json:"start,omitempty"`
				End       int    `json:"end,omitempty"`
				Url       string `json:"url,omitempty"`
				AudioText string `json:"audio_text,omitempty"`
				Result    struct {
					Suggestion string `json:"suggestion"`
					Scenes     struct {
						Antispam struct {
							Suggestion string `json:"suggestion"`
							Details    []struct {
								Suggestion string  `json:"suggestion"`
								Label      string  `json:"label"`
								Text       string  `json:"text"`
								Score      float64 `json:"score"`
							} `json:"details"`
						} `json:"antispam"`
					} `json:"scenes"`
				} `json:"result,omitempty"`
			} `json:"audio"`
		} `json:"items"`
	} `json:"data"`
}

type JobListRequest struct {
	Start  int64  `json:"start"`
	End    int64  `json:"end"`
	Status string `json:"status"`
	Limit  int    `json:"limit"`
	Marker string `json:"marker"`
}

type JobListResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Marker string `json:"marker"`
		Items  []struct {
			Id   string `json:"id"`
			Data struct {
				Id   string `json:"id"`
				Uri  string `json:"uri"`
				Info string `json:"info"`
			} `json:"data"`
			Params struct {
				HookUrl  string `json:"hook_url"`
				HookAuth bool   `json:"hook_auth"`
				Image    struct {
					IsOn          bool     `json:"is_on"`
					Scenes        []string `json:"scenes"`
					IntervalMsecs int      `json:"interval_msecs"`
					Saver         struct {
						Uid    int    `json:"uid"`
						Bucket string `json:"bucket"`
						Prefix string `json:"prefix"`
					} `json:"saver"`
					HookRule int `json:"hook_rule"`
				} `json:"image"`
			} `json:"params"`
			Message   string `json:"message"`
			Status    string `json:"status"`
			CreatedAt int    `json:"created_at"`
			UpdatedAt int    `json:"updated_at"`
		} `json:"items"`
	} `json:"data"`
}

type JobCreateRequest struct {
	Data   JobLiveData     `json:"data"`
	Params JobCreateParams `json:"params"`
}

type JobLiveData struct {
	ID   string `json:"ID"`
	Url  string `json:"uri"`
	Info string `json:"info"`
}

type JobCreateParams struct {
	Image    JobImage `json:"image"`
	HookUrl  string   `json:"hook_url"`
	HookAuth bool     `json:"hook_auth"`
}

type JobImage struct {
	IsOn          bool          `json:"is_on"`
	Scenes        []string      `json:"scenes"`
	IntervalMsecs int           `json:"interval_msecs"`
	Saver         JobImageSaver `json:"saver"`
	HookRule      int           `json:"hook_rule"`
}

type JobImageSaver struct {
	Bucket string `json:"bucket"`
	Prefix string `json:"prefix"`
}

type JobCreateResponse struct {
	RequestId string                `json:"request_id"` //请求ID
	Code      int                   `json:"code"`       //错误码，0 成功，其他失败
	Message   string                `json:"message"`    //错误信息
	Data      JobCreateResponseData `json:"data"`
}

type JobCreateResponseData struct {
	JobID string `json:"job"`
}

func (c *CensorServiceImpl) ImageBucketToUrl(url string) string {
	split := strings.Split(url, c.Bucket)
	return c.Addr + split[1]
}