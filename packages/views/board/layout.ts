import type { BoardView, Card, SessionView } from "@codedock/core/board";

export const BOARD_COL_WIDTH = 320;
export const BOARD_COL_GAP = 8;

export type BoardColumn = {
  id: string;
  title: string;
  card?: Card;
  sessions: SessionView[];
  ungrouped: boolean;
};

// visibleColumnCount 当前视口横向能放下几列，至少 1。
export function visibleColumnCount(width: number): number {
  if (width <= 0) {
    return 1;
  }
  return Math.max(1, Math.floor((width + BOARD_COL_GAP) / (BOARD_COL_WIDTH + BOARD_COL_GAP)));
}

// revealColumnCount 滑动到末尾时再露出一批，不超过总数。
export function revealColumnCount(shown: number, total: number, batch: number): number {
  const step = Math.max(1, batch);
  return Math.min(Math.max(0, total), Math.max(0, shown) + step);
}

// boardColumns 把已归组卡和未归组列收成横向列。
export function boardColumns(view: BoardView): BoardColumn[] {
  const cols: BoardColumn[] = (view.cards ?? []).map((card) => ({
    id: card.work.id,
    title: card.work.title || "未命名",
    card,
    sessions: card.sessions ?? [],
    ungrouped: false,
  }));
  cols.push({
    id: "ungrouped",
    title: "未分组",
    sessions: view.ungrouped ?? [],
    ungrouped: true,
  });
  return cols;
}

