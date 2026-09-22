import type { BoardView, Card, SessionView } from "@codedock/core/board";

export const BOARD_COL_MIN = 240;
export const BOARD_COL_MAX = 3;

export type BoardColumn = {
  id: string;
  title: string;
  card?: Card;
  sessions: SessionView[];
  ungrouped: boolean;
};

// columnsPerRow 按容器宽度限制一行列数，约 240px 一列，最多 3。
export function columnsPerRow(width: number): number {
  if (width <= 0) {
    return 1;
  }
  return Math.min(BOARD_COL_MAX, Math.max(1, Math.floor(width / BOARD_COL_MIN)));
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

// pageColumns 按当前页切出一排列。
export function pageColumns(columns: BoardColumn[], page: number, perPage: number): BoardColumn[] {
  const size = Math.max(1, perPage);
  const start = Math.max(0, page) * size;
  return columns.slice(start, start + size);
}

// pageCount 看板底部分页页数。
export function pageCount(total: number, perPage: number): number {
  const size = Math.max(1, perPage);
  return Math.max(1, Math.ceil(Math.max(0, total) / size));
}
