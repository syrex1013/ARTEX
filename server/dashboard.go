package server

import (
	"github.com/Autumn-27/artex/db"
	"net/http"
)

func (s *Server) dashboardFindings(w http.ResponseWriter, r *http.Request) {
	var total, critical, high, medium, low int
	if err := s.m.pg.QueryRowContext(r.Context(), `SELECT count(*),count(*) FILTER(WHERE severity='critical'),count(*) FILTER(WHERE severity='high'),count(*) FILTER(WHERE severity='medium'),count(*) FILTER(WHERE severity='low') FROM findings`).Scan(&total, &critical, &high, &medium, &low); err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	fs, _, err := s.m.pg.ListFindingsPage(db.FindingFilter{}, 1, 8)
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	items := make([]FindingDTO, 0, len(fs))
	for _, f := range fs {
		items = append(items, findingFromDB(f, nil))
	}
	writeJSON(w, 200, map[string]any{"total": total, "severity": map[string]int{"critical": critical, "high": high, "medium": medium, "low": low}, "recent": items})
}
func (s *Server) dashboardTasks(w http.ResponseWriter, r *http.Request) {
	s.collabJSON(w, r, `WITH alive AS (SELECT * FROM tasks WHERE deleted_at IS NULL AND archived_at IS NULL),
 recent AS (SELECT * FROM alive ORDER BY updated_at DESC,id DESC LIMIT 20),
 counts AS (SELECT CASE WHEN queued THEN 'queued' WHEN paused THEN 'paused' ELSE status END AS status,count(*) AS total FROM alive GROUP BY 1),
 usage AS (SELECT t.llm_profile_id,sum(COALESCE(a.input_tokens,0)) AS input_tokens,sum(COALESCE(a.output_tokens,0)) AS output_tokens,sum(COALESCE(a.cache_read_tokens,0)) AS cache_read_tokens,sum(COALESCE(a.cache_write_tokens,0)) AS cache_write_tokens FROM alive t LEFT JOIN activity a ON a.exploration_id=t.exploration_id AND a.kind='result' GROUP BY t.llm_profile_id),
 daily AS (SELECT t.llm_profile_id,to_char(a.created_at AT TIME ZONE 'UTC','YYYY-MM-DD') AS date,sum(a.input_tokens) AS input_tokens,sum(a.output_tokens) AS output_tokens,sum(a.cache_read_tokens) AS cache_read_tokens FROM alive t JOIN activity a ON a.exploration_id=t.exploration_id AND a.kind='result' GROUP BY 1,2
 UNION ALL SELECT c.llm_profile_id,to_char(a.created_at AT TIME ZONE 'UTC','YYYY-MM-DD'),sum(a.input_tokens),sum(a.output_tokens),sum(a.cache_read_tokens) FROM conversations c JOIN conversation_activities a ON a.conversation_id=c.id AND a.kind='result' GROUP BY 1,2)
 SELECT jsonb_build_object('total',(SELECT count(*) FROM alive),'counts',COALESCE((SELECT jsonb_object_agg(status,total) FROM counts),'{}'),
 'tasks',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',id::text,'name',name,'description',description,'goal',goal,'status',CASE WHEN queued THEN 'queued' WHEN paused THEN 'paused' ELSE status END,'created_at',created_at,'created_unix',extract(epoch FROM created_at),'last_activity_unix',extract(epoch FROM updated_at))) FROM recent),'[]'),
 'token_daily',COALESCE((SELECT jsonb_agg(daily) FROM daily),'[]'),
 'token_profiles',COALESCE((SELECT jsonb_agg(to_jsonb(usage)||jsonb_build_object('task_count',(SELECT count(*) FROM alive WHERE llm_profile_id IS NOT DISTINCT FROM usage.llm_profile_id))) FROM usage),'[]'))`)
}
