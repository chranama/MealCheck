from pathlib import Path

from PIL import Image, ImageDraw, ImageFont


WIDTH = 1200
HEIGHT = 627
UI_ROOT = Path(__file__).resolve().parents[1]
OUTPUT = UI_ROOT / "public" / "og-mealcheck.png"


def font(path: str, size: int) -> ImageFont.FreeTypeFont:
    return ImageFont.truetype(path, size=size)


SANS = "/System/Library/Fonts/Supplemental/Arial.ttf"
SANS_BOLD = "/System/Library/Fonts/Supplemental/Arial Bold.ttf"
MONO = "/System/Library/Fonts/SFNSMono.ttf"


def main() -> None:
    image = Image.new("RGB", (WIDTH, HEIGHT), "#f5f8f8")
    draw = ImageDraw.Draw(image)

    draw.rounded_rectangle(
        (72, 72, 1128, 555),
        radius=28,
        fill="#ffffff",
        outline="#c9dfe4",
        width=2,
    )

    draw.text((128, 130), "MealCheck", fill="#123a52", font=font(SANS_BOLD, 92))
    draw.text(
        (132, 230),
        "Public AI meal-plan verifier",
        fill="#1f6f8b",
        font=font(SANS_BOLD, 34),
    )
    draw.text(
        (132, 320),
        "Local-model normalization.",
        fill="#294a55",
        font=font(SANS_BOLD, 30),
    )
    draw.text(
        (132, 365),
        "Source-linked review. Deterministic checks.",
        fill="#294a55",
        font=font(SANS_BOLD, 30),
    )
    draw.text((132, 482), "mealcheck.dev", fill="#1f6f8b", font=font(MONO, 28))

    draw.rounded_rectangle(
        (770, 150, 1090, 482),
        radius=22,
        fill="#edf6f7",
        outline="#bed8df",
        width=2,
    )
    draw.text((815, 198), "DECISION", fill="#1b6577", font=font(MONO, 24))

    chips = [
        ((815, 240, 1045, 302), "#e1f5eb", "#2b8a62", "PASS"),
        ((815, 322, 1045, 384), "#fbefcf", "#8a6200", "WARN"),
        ((815, 404, 1045, 466), "#f7dfdb", "#9b2b1f", "BLOCK"),
    ]
    for rect, fill, ink, label in chips:
        draw.rounded_rectangle(rect, radius=12, fill=fill, outline=ink, width=2)
        draw.text((rect[0] + 34, rect[1] + 17), label, fill=ink, font=font(MONO, 28))

    image.save(OUTPUT)
    print(f"Wrote {OUTPUT}")


if __name__ == "__main__":
    main()
