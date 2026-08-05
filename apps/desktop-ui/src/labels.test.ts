// route 문자열 계약 테스트.
//
// route는 Go가 DB에 쓰고(notification.RouteSearchHistory), Rust 셸이 읽어 navigate
// 이벤트로 넘기고(tray.rs), 여기가 화면으로 해석한다. 세 언어를 이어 주는 것이 아무것도
// 없어서 한 곳만 바뀌면 조용히 어긋난다 — 알림을 눌러도 아무 일이 없는 식으로. Go 쪽은
// 마이그레이션 테스트가 지키고, 이쪽 끝을 지키는 것이 이 파일이다.

import { describe, expect, it } from "vitest";
import { ROUTES, resolveRoute, routeLabel } from "./labels";

describe("route 해석", () => {
  it("서버·Rust가 보내는 이름을 그대로 알아본다", () => {
    // 이 목록이 곧 계약이다. Go의 notification.RouteSearchHistory/RouteTodayReview,
    // Rust tray.rs의 ITEMS가 같은 문자열을 쓴다.
    for (const route of ["Search History", "Today Review", "Dashboard", "Settings"]) {
      expect(resolveRoute(route)).toBe(route);
    }
  });

  it("모르는 이름은 이동하지 않는다", () => {
    // 옛 이름은 마이그레이션 0002가 저장된 행까지 바꿨으므로 더 이상 받아 주지 않는다.
    // 엉뚱한 화면으로 가느니 가만히 있는 편이 낫다.
    expect(resolveRoute("Inbox")).toBeNull();
    expect(resolveRoute("")).toBeNull();
  });

  it("모든 화면에 한글 라벨이 있다", () => {
    // 라벨이 빠지면 그 탭만 영문으로 남는다(routeLabel은 모르는 값을 그대로 돌려준다).
    for (const route of ROUTES) {
      expect(routeLabel(route)).not.toBe(route);
    }
  });
});
