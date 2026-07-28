// Package rsrc 는 Windows 리소스를 만들어 COFF 오브젝트(`.syso`)로 내놓는다.
//
// Go 링커는 리소스 컴파일러를 대신하지 않는다. 받아들이는 것은 `.rsrc` 구역을
// 가진 COFF 오브젝트뿐이다. T1 스파이크가 `mc.exe`·`rc.exe`·`windres` 의존을
// 버리기로 정했으므로(docs/T1-spike.md 1장) 그 오브젝트를 여기서 직접 만든다.
//
// 다루는 리소스는 셋이다.
//
//   - MESSAGETABLE — 이벤트 뷰어가 이벤트 번호로 문구를 찾는 표. 없으면 기존
//     서비스의 과거 이벤트까지 "설명을 찾을 수 없습니다" 로 보인다.
//   - VERSIONINFO — 파일 속성 창의 버전 정보.
//   - ICON/GROUP_ICON, MANIFEST — 겉모습과 실행 수준. 원본과 같은 값을 쓴다.
package rsrc

import (
	"errors"
	"fmt"
	"sort"
)

// 리소스 종류 번호. WinUser.h 의 RT_* 다.
const (
	TypeIcon         uint16 = 3
	TypeMessageTable uint16 = 11
	TypeGroupIcon    uint16 = 14
	TypeVersion      uint16 = 16
	TypeManifest     uint16 = 24
)

// ErrEmpty 는 리소스가 하나도 없다는 뜻이다. 빈 `.syso` 는 링커를 헷갈리게
// 하므로 만들지 않는다.
var ErrEmpty = errors.New("no resources to write")

// Entry 는 리소스 하나다. 종류·이름·언어의 세 층으로 자리가 정해진다.
type Entry struct {
	Type     uint16
	Name     uint16
	Language uint16
	Data     []byte
}

// Set 은 한 실행 파일에 넣을 리소스 모음이다.
type Set struct {
	entries []Entry
}

// Add 는 리소스를 더한다. 같은 (종류, 이름, 언어) 를 두 번 넣으면 오류다.
func (s *Set) Add(e Entry) error {
	if len(e.Data) == 0 {
		return fmt.Errorf("resource type %d name %d language %d has no data", e.Type, e.Name, e.Language)
	}
	for _, have := range s.entries {
		if have.Type == e.Type && have.Name == e.Name && have.Language == e.Language {
			return fmt.Errorf("duplicate resource type %d name %d language %d", e.Type, e.Name, e.Language)
		}
	}
	s.entries = append(s.entries, e)
	return nil
}

// Entries 는 종류·이름·언어 오름차순으로 정렬한 사본이다.
//
// PE 명세는 리소스 디렉터리 항목이 정렬되어 있기를 요구한다. 로더가 이진
// 탐색으로 찾기 때문이다.
func (s *Set) Entries() []Entry {
	out := append([]Entry(nil), s.entries...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Language < b.Language
	})
	return out
}
