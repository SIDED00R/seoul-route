package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
)

// KakaoBaseURL 은 카카오 로컬 API 주소. 테스트에서 Server.KakaoBase 로 바꿔 끼운다.
const KakaoBaseURL = "https://dapi.kakao.com"

// kakaoFail 은 카카오 호출이 어디서 실패했는지다. 문구는 경로마다 다르므로 호출자가 정한다.
type kakaoFail int

const (
	kakaoOK         kakaoFail = iota
	kakaoBadRequest           // 요청을 만들지 못했다
	kakaoCallFailed           // 호출이 실패했거나 200 이 아니다
	kakaoBadBody              // 응답을 JSON 으로 읽지 못했다
)

func (s *Server) kakaoBase() string {
	if s.KakaoBase != "" {
		return s.KakaoBase
	}
	return KakaoBaseURL
}

// kakaoGet 은 카카오 로컬 API 를 호출해 JSON 을 out 에 담는다. op 는 로그에 남길 이름이다.
// url.Error 에는 검색어·좌표가 든 요청 URL 이 실리므로 껍질을 벗기고 원인만 남긴다.
func (s *Server) kakaoGet(ctx context.Context, op, path string, params url.Values, out any) kakaoFail {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.kakaoBase()+path+"?"+params.Encode(), nil)
	if err != nil {
		return kakaoBadRequest
	}
	req.Header.Set("Authorization", "KakaoAK "+s.KakaoKey)
	resp, err := s.HTTP.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		s.Log.Warn(op, "err", err)
		return kakaoCallFailed
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		s.Log.Warn(op, "status", resp.StatusCode)
		return kakaoCallFailed
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
		return kakaoBadBody
	}
	return kakaoOK
}
