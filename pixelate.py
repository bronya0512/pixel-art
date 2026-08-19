# -*- coding: utf-8 -*-
"""
图片转像素画脚本
支持自定义像素块大小（比例）、调色板颜色数量、输出尺寸等。

用法示例：
    # 按像素块大小 12 像素转换（默认）
    python pixelate.py input.jpg -s 12

    # 自定义像素块大小 + 输出路径
    python pixelate.py input.png -s 20 -o output.png

    # 限制调色板颜色数量（量化后像素画更像复古游戏风）
    python pixelate.py input.jpg -s 16 -c 32

    # 指定输出像素画的像素数量（如宽 64 个像素块），自动计算块大小
    python pixelate.py input.jpg -w 64

依赖：pip install pillow
"""

import argparse
import os
import sys

from PIL import Image


def pixelate_by_block(image: Image.Image, block_size: int, palette_colors: int = 0) -> Image.Image:
    """
    按块取平均色进行像素化。

    :param image: 输入图片（RGB）
    :param block_size: 像素块边长（像素），即每个色块的尺寸
    :param palette_colors: 调色板颜色数量，0 表示不量化（保留平均色）
    """
    w, h = image.size
    block = max(1, int(block_size))

    # 计算像素化后的小图尺寸（块数）
    cols = max(1, w // block)
    rows = max(1, h // block)

    # 缩放到块数尺寸，BOX 平均采样得到每个块的平均色
    small = image.resize((cols, rows), Image.Resampling.BOX)

    # 可选：量化到固定数量调色板
    if palette_colors and palette_colors > 0:
        small = small.quantize(colors=int(palette_colors), method=Image.Quantize.MEDIANCUT).convert("RGB")

    # 用最近邻放大回接近原始尺寸，形成清晰的像素块边缘
    result = small.resize((cols * block, rows * block), Image.Resampling.NEAREST)
    return result


def main():
    parser = argparse.ArgumentParser(description="图片转像素画脚本")
    parser.add_argument("input", help="输入图片路径")
    parser.add_argument("-o", "--output", help="输出图片路径（默认与输入同名 + _pixel）")
    parser.add_argument("-s", "--size", type=int, default=12, help="像素块边长（像素），默认 12")
    parser.add_argument("-w", "--width-blocks", type=int, default=0,
                        help="指定输出宽度有多少个像素块，会自动计算块大小（优先级高于 -s）")
    parser.add_argument("-c", "--colors", type=int, default=0,
                        help="调色板颜色数量，0 表示不量化（保留平均色），常用 8/16/32/64")
    args = parser.parse_args()

    if not os.path.exists(args.input):
        print(f"错误：找不到输入图片 {args.input}")
        sys.exit(1)

    image = Image.open(args.input).convert("RGB")
    w, h = image.size

    block_size = args.size
    if args.width_blocks and args.width_blocks > 0:
        block_size = max(1, w // args.width_blocks)

    result = pixelate_by_block(image, block_size, args.colors)

    if not args.output:
        base, ext = os.path.splitext(args.input)
        args.output = f"{base}_pixel.png"

    result.save(args.output)
    print(f"完成：原图 {w}x{h} -> 像素画 {result.size[0]}x{result.size[1]}（像素块 {block_size}px）")
    print(f"已保存到：{args.output}")


if __name__ == "__main__":
    main()
