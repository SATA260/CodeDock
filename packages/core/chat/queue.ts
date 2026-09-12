/** joinQueuedTexts 把待发正文 trim 后按换行拼成一条，中间不留空行。 */
export function joinQueuedTexts(texts: string[]): string {
  return texts
    .map((text) => text.trim())
    .filter((text) => text.length > 0)
    .join("\n");
}
