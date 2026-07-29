package main

const pathfindRadius = 25

var pathDirs = [8][2]int{
	{0, -1}, {1, -1}, {1, 0}, {1, 1},
	{0, 1}, {-1, 1}, {-1, 0}, {-1, -1},
}

// findPath 在格子上做 BFS 寻路（8 方向，禁止斜穿墙角），返回
// 起点之后、终点含的路点序列；无路径或终点超出起点周围
// 50×50 搜索区域时返回 nil。
func findPath(canWalk func(x, y int) bool, startX, startY, endX, endY int) [][2]int {
	if startX == endX && startY == endY {
		return nil
	}
	minX, maxX := startX-pathfindRadius, startX+pathfindRadius
	minY, maxY := startY-pathfindRadius, startY+pathfindRadius
	if endX < minX || endX > maxX || endY < minY || endY > maxY {
		return nil
	}
	w := maxX - minX + 1
	size := w * (maxY - minY + 1)
	idxOf := func(x, y int) int { return (y-minY)*w + (x - minX) }

	parent := make([]int, size)
	for i := range parent {
		parent[i] = -1
	}
	start := idxOf(startX, startY)
	parent[start] = start
	queue := make([]int, 0, 256)
	queue = append(queue, start)

	found := false
	for head := 0; head < len(queue) && !found; head++ {
		cur := queue[head]
		cx := cur%w + minX
		cy := cur/w + minY
		for _, d := range pathDirs {
			nx, ny := cx+d[0], cy+d[1]
			if nx < minX || nx > maxX || ny < minY || ny > maxY {
				continue
			}
			ni := idxOf(nx, ny)
			if parent[ni] != -1 || !canWalk(nx, ny) {
				continue
			}
			if d[0] != 0 && d[1] != 0 && (!canWalk(cx+d[0], cy) || !canWalk(cx, cy+d[1])) {
				continue
			}
			parent[ni] = cur
			if nx == endX && ny == endY {
				found = true
				break
			}
			queue = append(queue, ni)
		}
	}
	if !found {
		return nil
	}

	var rev [][2]int
	for cur := idxOf(endX, endY); cur != start; cur = parent[cur] {
		rev = append(rev, [2]int{cur%w + minX, cur/w + minY})
	}
	path := make([][2]int, len(rev))
	for i, p := range rev {
		path[len(rev)-1-i] = p
	}
	return path
}
