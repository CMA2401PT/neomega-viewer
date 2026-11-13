export interface Tile { x: number; z: number; kind: "overview"; key: string }

export class MapTiles<T extends { close(): void }> {
  // 两种视图共享图片；废弃请求占着名额直到真正返回，切换代次后迟到的图片立即释放。
  readonly images = new Map<string, T>();
  readonly failed = new Set<string>();
  private wanted: Tile[] = [];
  private active = new Map<string, AbortController>();
  private epoch = 0;
  private readonly load: (tile: Tile, signal: AbortSignal) => Promise<T>;
  private readonly error: (error: unknown) => void;
  constructor(
    load: (tile: Tile, signal: AbortSignal) => Promise<T>,
    error: (error: unknown) => void,
  ) {
    this.load = load;
    this.error = error;
  }
  update(tiles: Tile[]): void {
    // 平移后取消离开视口的下载，把名额留给当前可见图块。
    this.wanted = tiles;
    const keys = new Set(tiles.map((tile) => tile.key));
    for (const [key, controller] of this.active) {
      if (!keys.has(key)) controller.abort();
    }
    this.pump();
  }
  reset(): void {
    // 切换存档时释放图片，并让迟到响应无法混入新地图。
    this.epoch++;
    for (const controller of this.active.values()) controller.abort();
    for (const image of this.images.values()) image.close();
    this.images.clear();
    this.failed.clear();
    this.wanted = [];
  }
  private pump(): void {
    // 下载完成立即补位，取消的请求归还名额前仍计入并发数。
    for (const tile of this.wanted) {
      if (this.active.size >= 2) break;
      if (
        this.images.has(tile.key)
        || this.active.has(tile.key)
        || this.failed.has(tile.key)
      ) continue;
      const controller = new AbortController();
      const epoch = this.epoch;
      this.active.set(tile.key, controller);
      void this.load(tile, controller.signal).then((image) => {
        if (controller.signal.aborted || epoch !== this.epoch) {
          image.close();
          return;
        }
        this.images.set(tile.key, image);
        const wanted = new Set(this.wanted.map((entry) => entry.key));
        for (const [key, previous] of this.images) {
          if (this.images.size <= 256) break;
          if (!wanted.has(key)) {
            previous.close();
            this.images.delete(key);
          }
        }
      }).catch((error: unknown) => {
        if (!controller.signal.aborted && epoch === this.epoch) {
          this.failed.add(tile.key);
          this.error(error);
        }
      }).finally(() => {
        if (this.active.get(tile.key) === controller) this.active.delete(tile.key);
        this.pump();
      });
    }
  }
}

