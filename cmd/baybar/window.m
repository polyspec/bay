#import <Cocoa/Cocoa.h>
#include "window.h"
#include "_cgo_export.h"

// The window is a source list: a fixed-width sidebar for navigation and a pane
// showing one subject. Sizes come from design/Mockups.dc.html, which states
// them in points.
static const CGFloat kSidebarWidth = 216;   // .side
static const CGFloat kNameWidth = 96;       // .name
static const CGFloat kNameWideWidth = 140;  // .name.w140
static const CGFloat kKeyWidth = 116;       // .kv b
static const CGFloat kDotSize = 9;          // .dot
static const CGFloat kCodePad = 14;         // .logpane padding
static const CGFloat kControlGap = 8;       // between two controls in one row
static const CGFloat kSubDotSize = 3;       // .dot on a service under its project
// Every sidebar row is one line of 13 point text plus its padding. Stating it
// keeps a service row the height of the project above it, whose dot is larger.
static const CGFloat kSideRowHeight = 27;   // .sitem
static const CGFloat kBeaconSize = 11;      // .beacon
static const CGFloat kBeaconHalo = 4;       // .beacon halo
static const CGFloat kSubIndent = 18;       // one service under its project
static const CGFloat kCardLine = 15;        // one line of type in a card
static const CGFloat kCardPad = 9;          // the space at a card's edge, which
                                            // is also the space between two of
                                            // its rows


// gLabels holds the words Go supplies for the controls this file builds, so the
// whole window reads in one language.
static NSDictionary *gLabels = nil;

static NSString *label(NSString *name, NSString *fallback) {
  NSString *v = gLabels[name];
  return v.length ? v : fallback;
}

static NSArray<NSString *> *appearanceLabels(void) {
  return @[label(@"auto", @"Auto"), label(@"dark", @"Dark"), label(@"light", @"Light")];
}

static NSColor *dotColor(NSString *state) {
  if ([state isEqualToString:@"on"])   return [NSColor systemGreenColor];
  if ([state isEqualToString:@"warn"]) return [NSColor systemOrangeColor];
  if ([state isEqualToString:@"bad"])  return [NSColor systemRedColor];
  // "off" and anything unset mean nothing is running, which is not a fault.
  return [NSColor tertiaryLabelColor];
}

// A colour that resolves against the appearance the view draws in. Layer
// colours do not follow an appearance change on their own, so every layer-backed
// view here redraws from viewDidChangeEffectiveAppearance.
static NSColor *dynamicColor(NSColor *light, NSColor *dark) {
  return [NSColor colorWithName:nil dynamicProvider:^NSColor *(NSAppearance *a) {
    NSAppearanceName name = [a bestMatchFromAppearancesWithNames:
        @[NSAppearanceNameAqua, NSAppearanceNameDarkAqua]];
    return [name isEqualToString:NSAppearanceNameDarkAqua] ? dark : light;
  }];
}

// A card's outline and the hairline between its rows are two different weights.
// NSColor offers one separator, so both are stated here.
static NSColor *cardBorderColor(void) {
  static NSColor *c;
  static dispatch_once_t once;
  dispatch_once(&once, ^{
    c = dynamicColor([NSColor colorWithWhite:0 alpha:0.215],
                     [NSColor colorWithWhite:1 alpha:0.145]);
  });
  return c;
}

static NSColor *hairlineColor(void) {
  static NSColor *c;
  static dispatch_once_t once;
  dispatch_once(&once, ^{
    c = dynamicColor([NSColor colorWithWhite:0 alpha:0.090],
                     [NSColor colorWithWhite:1 alpha:0.080]);
  });
  return c;
}

// The ring drawn inside a status dot.
static NSColor *dotRingColor(void) {
  static NSColor *c;
  static dispatch_once_t once;
  dispatch_once(&once, ^{
    c = dynamicColor([NSColor colorWithWhite:0 alpha:0.10],
                     [NSColor colorWithWhite:1 alpha:0.12]);
  });
  return c;
}

// The halo around the verdict beacon, in the beacon's own colour.
static CGFloat haloAlpha(NSView *view) {
  NSAppearanceName name = [view.effectiveAppearance bestMatchFromAppearancesWithNames:
      @[NSAppearanceNameAqua, NSAppearanceNameDarkAqua]];
  return [name isEqualToString:NSAppearanceNameDarkAqua] ? 0.16 : 0.20;
}

// NSView places subviews from the bottom left, so a stack inside a scroll view
// is laid out from the bottom. A flipped view places the origin at the top
// left.
@interface FlippedView : NSView
@end
@implementation FlippedView
- (BOOL)isFlipped { return YES; }
@end


// Chip is the bordered pill that labels a row, such as a project's domain.
@interface Chip : NSView
@property(strong) NSTextField *label;
@end

@implementation Chip
- (instancetype)initWithText:(NSString *)text {
  if ((self = [super initWithFrame:NSZeroRect])) {
    self.wantsLayer = YES;
    self.layer.cornerRadius = 5;
    self.layer.borderWidth = 1;

    self.label = [NSTextField labelWithString:text];
    self.label.font = [NSFont systemFontOfSize:11];
    self.label.textColor = [NSColor secondaryLabelColor];
    self.label.translatesAutoresizingMaskIntoConstraints = NO;
    [self addSubview:self.label];
    [NSLayoutConstraint activateConstraints:@[
      [self.label.leadingAnchor constraintEqualToAnchor:self.leadingAnchor constant:6],
      [self.label.trailingAnchor constraintEqualToAnchor:self.trailingAnchor constant:-6],
      [self.label.topAnchor constraintEqualToAnchor:self.topAnchor constant:1],
      [self.label.bottomAnchor constraintEqualToAnchor:self.bottomAnchor constant:-1],
    ]];
  }
  return self;
}
- (void)updateLayer {
  self.layer.backgroundColor = [NSColor clearColor].CGColor;
  self.layer.borderColor = cardBorderColor().CGColor;
}
- (void)viewDidChangeEffectiveAppearance { [self setNeedsDisplay:YES]; }
@end

// DotView draws a status dot. The verdict beacon is the same dot with a halo in
// its own colour, and a dot in a selected sidebar row carries a white ring.
@interface DotView : NSView
@property(strong) NSColor *color;
@property(assign) CGFloat diameter;
@property(assign) CGFloat halo;
@property(assign) BOOL selected;
+ (instancetype)dot:(NSString *)state;
+ (instancetype)dot:(NSString *)state size:(CGFloat)d;
+ (instancetype)beacon:(NSString *)state;
@end

@implementation DotView
+ (instancetype)dot:(NSString *)state { return [self dot:state size:kDotSize]; }
+ (instancetype)dot:(NSString *)state size:(CGFloat)d {
  DotView *v = [[DotView alloc] initWithFrame:NSZeroRect];
  v.color = dotColor(state);
  v.diameter = d;
  [v.widthAnchor constraintEqualToConstant:d].active = YES;
  [v.heightAnchor constraintEqualToConstant:d].active = YES;
  return v;
}
+ (instancetype)beacon:(NSString *)state {
  CGFloat side = kBeaconSize + 2 * kBeaconHalo;
  DotView *v = [[DotView alloc] initWithFrame:NSZeroRect];
  v.color = dotColor(state);
  v.diameter = kBeaconSize;
  v.halo = kBeaconHalo;
  [v.widthAnchor constraintEqualToConstant:side].active = YES;
  [v.heightAnchor constraintEqualToConstant:side].active = YES;
  return v;
}
- (void)drawRect:(NSRect)r {
  CGFloat d = self.diameter > 0 ? self.diameter : kDotSize;
  NSRect box = NSMakeRect(NSMidX(self.bounds) - d/2, NSMidY(self.bounds) - d/2, d, d);
  if (self.halo > 0) {
    [[self.color colorWithAlphaComponent:haloAlpha(self)] setFill];
    [[NSBezierPath bezierPathWithOvalInRect:NSInsetRect(box, -self.halo, -self.halo)] fill];
  }
  [self.color setFill];
  [[NSBezierPath bezierPathWithOvalInRect:box] fill];
  // The ring is a hairline around a full size dot. A smaller dot has no room
  // for it, so it is drawn as a solid mark.
  if (d >= kDotSize) {
    NSColor *ring = self.selected ? [NSColor colorWithWhite:1 alpha:0.4] : dotRingColor();
    [ring setStroke];
    NSBezierPath *inner = [NSBezierPath bezierPathWithOvalInRect:NSInsetRect(box, 0.5, 0.5)];
    inner.lineWidth = 1;
    [inner stroke];
  }
}
- (void)viewDidChangeEffectiveAppearance { [self setNeedsDisplay:YES]; }
@end

// Card is a rounded container holding rows separated by hairlines.
@interface Card : NSView
@end
@implementation Card
- (instancetype)initWithFrame:(NSRect)f {
  if ((self = [super initWithFrame:f])) {
    self.wantsLayer = YES;
    self.layer.cornerRadius = 10;
    self.layer.borderWidth = 1;
  }
  return self;
}
- (BOOL)isFlipped { return YES; }
- (void)updateLayer {
  self.layer.backgroundColor = [NSColor controlBackgroundColor].CGColor;
  self.layer.borderColor = cardBorderColor().CGColor;
}
- (void)viewDidChangeEffectiveAppearance { [self setNeedsDisplay:YES]; }
@end

// Hairline separates two rows inside a card. It is fainter than the card's own
// outline.
@interface Hairline : NSView
// Strong draws the line at the weight a card is outlined with, for the edge
// between the sidebar and the pane.
@property(assign) BOOL strong;
@end
@implementation Hairline
- (void)updateLayer {
  self.layer.backgroundColor =
      self.strong ? cardBorderColor().CGColor : hairlineColor().CGColor;
}
- (void)viewDidChangeEffectiveAppearance { [self setNeedsDisplay:YES]; }
@end

static NSColor *hex(uint32_t rgb) {
  return [NSColor colorWithSRGBRed:((rgb >> 16) & 0xFF) / 255.0
                             green:((rgb >> 8) & 0xFF) / 255.0
                              blue:(rgb & 0xFF) / 255.0
                             alpha:1];
}

// TintedCard is the banner's background: a card in a warning or fault colour
// rather than the control background.
@interface TintedCard : Card
@property(strong) NSColor *fill;
@property(strong) NSColor *edge;
@end
@implementation TintedCard
- (void)updateLayer {
  self.layer.backgroundColor = self.fill.CGColor;
  self.layer.borderColor = self.edge.CGColor;
}
@end

// CodePane is the background a log tail is drawn on, darker than the card that
// holds it.
@interface CodePane : NSView
@end
@implementation CodePane
- (instancetype)initWithFrame:(NSRect)f {
  if ((self = [super initWithFrame:f])) self.wantsLayer = YES;
  return self;
}
- (BOOL)isFlipped { return YES; }
- (void)updateLayer {
  self.layer.backgroundColor =
      dynamicColor(hex(0xF7F8F9), hex(0x191A1B)).CGColor;
}
- (void)viewDidChangeEffectiveAppearance { [self setNeedsDisplay:YES]; }
@end

@class PanelController;

// ClickableRow is a sidebar entry. NSButton does not support this layout, so
// the view handles the click.
@interface ClickableRow : NSView
@property(assign) NSInteger action;
@property(assign) BOOL selected;
@property(weak) PanelController *owner;
@end

@interface PanelController : NSObject <NSWindowDelegate>
@property(strong) NSWindow *window;
@property(strong) NSStackView *sidebar;
@property(strong) NSStackView *content;
@property(strong) NSStackView *headerBar;
@property(strong) NSTextField *headerTitle;
@property(strong) NSTextField *headerSubtitle;
@property(strong) NSSegmentedControl *appearance;
// The Settings screen shows the same control. It is rebuilt with the pane, so
// the reference is dropped on every render.
@property(weak) NSSegmentedControl *bodyAppearance;
@property(strong) NSMutableArray<NSString *> *actionIds;
- (void)fire:(NSInteger)index;
@end

@implementation ClickableRow
- (BOOL)isFlipped { return YES; }
- (void)mouseDown:(NSEvent *)e { [self.owner fire:self.action]; }
- (void)resetCursorRects {
  [self addCursorRect:self.bounds cursor:[NSCursor pointingHandCursor]];
}
- (void)updateLayer {
  self.layer.backgroundColor = self.selected
      ? [NSColor controlAccentColor].CGColor : [NSColor clearColor].CGColor;
}
- (BOOL)allowsVibrancy { return NO; }
- (void)viewDidChangeEffectiveAppearance { [self setNeedsDisplay:YES]; }
@end

@implementation PanelController

+ (instancetype)shared {
  static PanelController *c;
  static dispatch_once_t once;
  dispatch_once(&once, ^{ c = [PanelController new]; });
  return c;
}

#pragma mark - appearance

// The appearance choice is stored in the defaults database. "auto" applies the
// system appearance.
- (void)applyStoredAppearance {
  NSString *choice = [[NSUserDefaults standardUserDefaults] stringForKey:@"appearance"] ?: @"auto";
  NSInteger index = 0;
  NSAppearance *appearance = nil;
  if ([choice isEqualToString:@"dark"]) {
    index = 1;
    appearance = [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua];
  } else if ([choice isEqualToString:@"light"]) {
    index = 2;
    appearance = [NSAppearance appearanceNamed:NSAppearanceNameAqua];
  }
  NSApp.appearance = appearance;
  self.appearance.selectedSegment = index;
  self.bodyAppearance.selectedSegment = index;
}

// appearanceControl returns the Auto/Dark/Light control for the Settings
// screen. It writes the same stored choice as the one in the title bar.
- (NSSegmentedControl *)appearanceControl {
  NSSegmentedControl *c =
      [NSSegmentedControl segmentedControlWithLabels:appearanceLabels()
                                        trackingMode:NSSegmentSwitchTrackingSelectOne
                                              target:self
                                              action:@selector(appearanceChanged:)];
  c.controlSize = NSControlSizeSmall;
  c.font = [NSFont systemFontOfSize:11];
  c.selectedSegment = self.appearance.selectedSegment;
  self.bodyAppearance = c;
  return c;
}

- (void)appearanceChanged:(NSSegmentedControl *)sender {
  NSArray *choices = @[@"auto", @"dark", @"light"];
  NSString *choice = choices[MAX(0, MIN(2, sender.selectedSegment))];
  [[NSUserDefaults standardUserDefaults] setObject:choice forKey:@"appearance"];
  [self applyStoredAppearance];
}

#pragma mark - chrome

- (void)build {
  if (self.window) return;
  self.actionIds = [NSMutableArray array];

  self.window = [[NSWindow alloc]
      initWithContentRect:NSMakeRect(0, 0, 940, 620)
                styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable |
                          NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable
                  backing:NSBackingStoreBuffered
                    defer:NO];
  self.window.title = @"bay";
  self.window.releasedWhenClosed = NO;
  self.window.delegate = self;
  self.window.minSize = NSMakeSize(760, 420);
  [self.window center];

  // A titlebar accessory places the control at the title bar's trailing
  // edge.
  self.appearance = [NSSegmentedControl segmentedControlWithLabels:appearanceLabels()
                                                     trackingMode:NSSegmentSwitchTrackingSelectOne
                                                           target:self
                                                           action:@selector(appearanceChanged:)];
  self.appearance.controlSize = NSControlSizeSmall;
  self.appearance.font = [NSFont systemFontOfSize:11];
  // A titlebar accessory is laid out from its frame, so the size is set before
  // it is added.
  [self.appearance sizeToFit];
  CGFloat w = NSWidth(self.appearance.frame) + 20;
  NSView *accessory = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, w, 32)];
  self.appearance.translatesAutoresizingMaskIntoConstraints = NO;
  [accessory addSubview:self.appearance];
  [NSLayoutConstraint activateConstraints:@[
    [self.appearance.trailingAnchor constraintEqualToAnchor:accessory.trailingAnchor constant:-14],
    [self.appearance.centerYAnchor constraintEqualToAnchor:accessory.centerYAnchor],
  ]];
  NSTitlebarAccessoryViewController *acc = [NSTitlebarAccessoryViewController new];
  acc.view = accessory;
  acc.layoutAttribute = NSLayoutAttributeRight;
  [self.window addTitlebarAccessoryViewController:acc];
  [self applyStoredAppearance];

  // Sidebar.
  self.sidebar = [NSStackView new];
  self.sidebar.orientation = NSUserInterfaceLayoutOrientationVertical;
  self.sidebar.alignment = NSLayoutAttributeLeading;
  self.sidebar.spacing = 2;
  self.sidebar.edgeInsets = NSEdgeInsetsMake(10, 0, 10, 0);
  self.sidebar.translatesAutoresizingMaskIntoConstraints = NO;

  NSVisualEffectView *sideBg = [NSVisualEffectView new];
  sideBg.material = NSVisualEffectMaterialSidebar;
  sideBg.blendingMode = NSVisualEffectBlendingModeBehindWindow;
  sideBg.translatesAutoresizingMaskIntoConstraints = NO;
  // The sidebar scrolls when its rows are taller than the window. A project with
  // twelve services pushed the rows below it out of the window with no way to
  // reach them.
  NSScrollView *sideScroll = [NSScrollView new];
  sideScroll.hasVerticalScroller = YES;
  sideScroll.autohidesScrollers = YES;
  sideScroll.drawsBackground = NO;
  sideScroll.translatesAutoresizingMaskIntoConstraints = NO;
  NSView *sideDoc = [FlippedView new];
  sideDoc.translatesAutoresizingMaskIntoConstraints = NO;
  [sideDoc addSubview:self.sidebar];
  sideScroll.documentView = sideDoc;
  [sideBg addSubview:sideScroll];
  [NSLayoutConstraint activateConstraints:@[
    [sideBg.widthAnchor constraintEqualToConstant:kSidebarWidth],
    [sideScroll.topAnchor constraintEqualToAnchor:sideBg.topAnchor],
    [sideScroll.leadingAnchor constraintEqualToAnchor:sideBg.leadingAnchor],
    [sideScroll.trailingAnchor constraintEqualToAnchor:sideBg.trailingAnchor],
    [sideScroll.bottomAnchor constraintEqualToAnchor:sideBg.bottomAnchor],
    [self.sidebar.topAnchor constraintEqualToAnchor:sideDoc.topAnchor],
    [self.sidebar.leadingAnchor constraintEqualToAnchor:sideDoc.leadingAnchor],
    [self.sidebar.trailingAnchor constraintEqualToAnchor:sideDoc.trailingAnchor],
    [self.sidebar.bottomAnchor constraintEqualToAnchor:sideDoc.bottomAnchor],
    [sideDoc.widthAnchor constraintEqualToAnchor:sideScroll.contentView.widthAnchor],
  ]];

  // Detail header.
  self.headerTitle = [NSTextField labelWithString:@""];
  self.headerTitle.font = [NSFont systemFontOfSize:20 weight:NSFontWeightSemibold];
  self.headerSubtitle = [NSTextField labelWithString:@""];
  self.headerSubtitle.font = [NSFont systemFontOfSize:12];
  self.headerSubtitle.textColor = [NSColor secondaryLabelColor];
  self.headerSubtitle.lineBreakMode = NSLineBreakByTruncatingHead;
  [self.headerSubtitle setContentCompressionResistancePriority:NSLayoutPriorityDefaultLow
                                                forOrientation:NSLayoutConstraintOrientationHorizontal];

  self.headerBar = [NSStackView new];
  self.headerBar.orientation = NSUserInterfaceLayoutOrientationHorizontal;
  self.headerBar.alignment = NSLayoutAttributeCenterY;
  self.headerBar.spacing = 12;
  self.headerBar.edgeInsets = NSEdgeInsetsMake(18, 24, 14, 24);
  self.headerBar.translatesAutoresizingMaskIntoConstraints = NO;

  NSBox *headerLine = [NSBox new];
  headerLine.boxType = NSBoxSeparator;
  headerLine.translatesAutoresizingMaskIntoConstraints = NO;

  // Detail body.
  self.content = [NSStackView new];
  self.content.orientation = NSUserInterfaceLayoutOrientationVertical;
  self.content.alignment = NSLayoutAttributeLeading;
  self.content.spacing = 18;
  self.content.translatesAutoresizingMaskIntoConstraints = NO;

  NSScrollView *scroll = [NSScrollView new];
  scroll.hasVerticalScroller = YES;
  scroll.drawsBackground = NO;
  scroll.translatesAutoresizingMaskIntoConstraints = NO;
  NSView *doc = [FlippedView new];
  doc.translatesAutoresizingMaskIntoConstraints = NO;
  [doc addSubview:self.content];
  scroll.documentView = doc;

  NSView *detail = [NSView new];
  detail.translatesAutoresizingMaskIntoConstraints = NO;
  [detail addSubview:self.headerBar];
  [detail addSubview:headerLine];
  [detail addSubview:scroll];

  // The sidebar and the pane meet on a line, the way the cards are outlined.
  Hairline *sideEdge = [Hairline new];
  sideEdge.strong = YES;
  sideEdge.wantsLayer = YES;
  sideEdge.translatesAutoresizingMaskIntoConstraints = NO;
  [sideEdge.widthAnchor constraintEqualToConstant:1].active = YES;

  NSView *root = [NSView new];
  [root addSubview:sideBg];
  [root addSubview:sideEdge];
  [root addSubview:detail];
  sideBg.translatesAutoresizingMaskIntoConstraints = NO;
  self.window.contentView = root;

  [NSLayoutConstraint activateConstraints:@[
    [sideBg.topAnchor constraintEqualToAnchor:root.topAnchor],
    [sideBg.bottomAnchor constraintEqualToAnchor:root.bottomAnchor],
    [sideBg.leadingAnchor constraintEqualToAnchor:root.leadingAnchor],

    [sideEdge.leadingAnchor constraintEqualToAnchor:sideBg.trailingAnchor],
    [sideEdge.topAnchor constraintEqualToAnchor:root.topAnchor],
    [sideEdge.bottomAnchor constraintEqualToAnchor:root.bottomAnchor],

    [detail.leadingAnchor constraintEqualToAnchor:sideEdge.trailingAnchor],
    [detail.topAnchor constraintEqualToAnchor:root.topAnchor],
    [detail.bottomAnchor constraintEqualToAnchor:root.bottomAnchor],
    [detail.trailingAnchor constraintEqualToAnchor:root.trailingAnchor],

    [self.headerBar.topAnchor constraintEqualToAnchor:detail.topAnchor],
    [self.headerBar.leadingAnchor constraintEqualToAnchor:detail.leadingAnchor],
    [self.headerBar.trailingAnchor constraintEqualToAnchor:detail.trailingAnchor],
    [headerLine.topAnchor constraintEqualToAnchor:self.headerBar.bottomAnchor],
    [headerLine.leadingAnchor constraintEqualToAnchor:detail.leadingAnchor],
    [headerLine.trailingAnchor constraintEqualToAnchor:detail.trailingAnchor],

    [scroll.topAnchor constraintEqualToAnchor:headerLine.bottomAnchor],
    [scroll.leadingAnchor constraintEqualToAnchor:detail.leadingAnchor],
    [scroll.trailingAnchor constraintEqualToAnchor:detail.trailingAnchor],
    [scroll.bottomAnchor constraintEqualToAnchor:detail.bottomAnchor],

    [self.content.topAnchor constraintEqualToAnchor:doc.topAnchor constant:18],
    [self.content.leadingAnchor constraintEqualToAnchor:doc.leadingAnchor constant:24],
    [self.content.trailingAnchor constraintEqualToAnchor:doc.trailingAnchor constant:-24],
    [self.content.bottomAnchor constraintEqualToAnchor:doc.bottomAnchor constant:-22],
    [doc.widthAnchor constraintEqualToAnchor:scroll.contentView.widthAnchor],
  ]];
}

#pragma mark - pieces

- (void)fire:(NSInteger)index {
  if (index < 0 || index >= (NSInteger)self.actionIds.count) return;
  goUIAction((char *)[self.actionIds[index] UTF8String]);
}

- (NSInteger)claim:(NSString *)action {
  [self.actionIds addObject:action ?: @""];
  return (NSInteger)self.actionIds.count - 1;
}

- (void)clicked:(NSButton *)sender { [self fire:sender.tag]; }

// A switch delivers its action the same way a button does. The view model
// carries the new state in the identifier, so the handler does not have to read
// the control back.
- (void)switched:(NSSwitch *)sender { [self fire:sender.tag]; }

// A segmented control delivers its identifier with the chosen index appended.
- (void)segmentChanged:(NSSegmentedControl *)sender {
  if (sender.tag < 0 || sender.tag >= (NSInteger)self.actionIds.count) return;
  NSString *action = [NSString stringWithFormat:@"%@:%ld", self.actionIds[sender.tag],
                                                (long)sender.selectedSegment];
  goUIAction((char *)[action UTF8String]);
}

// The mockup draws four button weights. A push button is the native control for
// all four, so the weight is carried by the fill and the label colour: the
// screen's main action takes the accent colour, a secondary action keeps the
// standard bezel with a quieter label, and a disabled one is drawn by AppKit.
- (NSButton *)buttonFor:(NSDictionary *)spec {
  NSButton *b = [NSButton buttonWithTitle:spec[@"title"] ?: @"" target:self action:@selector(clicked:)];
  b.bezelStyle = NSBezelStyleRounded;
  b.controlSize = NSControlSizeRegular;
  b.font = [NSFont systemFontOfSize:12];
  b.enabled = ![spec[@"disabled"] boolValue];
  NSString *style = spec[@"style"] ?: @"";
  if (b.enabled) {
    if ([style isEqualToString:@"hero"]) {
      b.bezelColor = [NSColor controlAccentColor];
      b.contentTintColor = [NSColor whiteColor];
      b.font = [NSFont systemFontOfSize:12 weight:NSFontWeightMedium];
    } else if ([style isEqualToString:@"quiet"]) {
      b.contentTintColor = [NSColor secondaryLabelColor];
    }
  }
  b.tag = [self claim:spec[@"id"]];
  return b;
}

- (NSView *)hairline {
  Hairline *v = [Hairline new];
  v.wantsLayer = YES;
  [v.heightAnchor constraintEqualToConstant:1].active = YES;
  return v;
}

- (NSView *)chip:(NSString *)text {
  return [[Chip alloc] initWithText:text];
}

#pragma mark - sidebar

- (NSView *)sidebarGroup:(NSDictionary *)group {
  NSStackView *box = [NSStackView new];
  box.orientation = NSUserInterfaceLayoutOrientationVertical;
  box.alignment = NSLayoutAttributeLeading;
  box.spacing = 2;

  NSTextField *title = [NSTextField labelWithString:group[@"title"] ?: @""];
  title.font = [NSFont systemFontOfSize:11 weight:NSFontWeightSemibold];
  title.textColor = [NSColor tertiaryLabelColor];
  NSView *titleWrap = [NSView new];
  title.translatesAutoresizingMaskIntoConstraints = NO;
  [titleWrap addSubview:title];
  [NSLayoutConstraint activateConstraints:@[
    [title.leadingAnchor constraintEqualToAnchor:titleWrap.leadingAnchor constant:16],
    [title.topAnchor constraintEqualToAnchor:titleWrap.topAnchor constant:12],
    [titleWrap.bottomAnchor constraintEqualToAnchor:title.bottomAnchor constant:4],
    [titleWrap.trailingAnchor constraintGreaterThanOrEqualToAnchor:title.trailingAnchor constant:16],
  ]];
  [box addArrangedSubview:titleWrap];

  for (NSDictionary *item in group[@"items"]) {
    BOOL selected = [item[@"selected"] boolValue];
    // A sub row is one service inside its project, indented under it.
    BOOL sub = [item[@"sub"] boolValue];
    ClickableRow *row = [ClickableRow new];
    row.owner = self;
    row.selected = selected;
    row.wantsLayer = YES;
    row.layer.cornerRadius = 6;
    row.action = [self claim:item[@"id"]];

    NSStackView *line = [NSStackView new];
    line.orientation = NSUserInterfaceLayoutOrientationHorizontal;
    line.alignment = NSLayoutAttributeCenterY;
    line.spacing = 9;
    line.edgeInsets = NSEdgeInsetsMake(6, sub ? 8 + kSubIndent : 8, 6, 8);
    line.translatesAutoresizingMaskIntoConstraints = NO;


    // Every row carries a dot, so the names stay in one column. A row with
    // nothing running carries the off colour rather than no dot at all. A
    // service sits under its project, so its dot is the smaller of the two.
    DotView *d = [DotView dot:item[@"dot"] ?: @"" size:sub ? kSubDotSize : kDotSize];
    d.selected = selected;
    [line addArrangedSubview:d];
    NSTextField *label = [NSTextField labelWithString:item[@"label"] ?: @""];
    // A service row keeps the type size of the project above it, so every row
    // in the sidebar is the same height. Indent, colour and a half-size dot
    // carry the level instead.
    label.font = [NSFont systemFontOfSize:13];
    label.lineBreakMode = NSLineBreakByTruncatingTail;
    if (sub) label.textColor = [NSColor secondaryLabelColor];
    if (selected) label.textColor = [NSColor whiteColor];
    [line addArrangedSubview:label];

    NSView *spacer = [NSView new];
    [spacer setContentHuggingPriority:1 forOrientation:NSLayoutConstraintOrientationHorizontal];
    [line addArrangedSubview:spacer];

    NSString *count = item[@"count"];
    if (count.length) {
      NSTextField *c = [NSTextField labelWithString:count];
      c.font = [NSFont monospacedDigitSystemFontOfSize:11 weight:NSFontWeightRegular];
      c.textColor = selected ? [[NSColor whiteColor] colorWithAlphaComponent:0.75]
                             : [NSColor secondaryLabelColor];
      [line addArrangedSubview:c];
    }

    [row addSubview:line];
    [row.heightAnchor constraintEqualToConstant:kSideRowHeight].active = YES;
    [NSLayoutConstraint activateConstraints:@[
      [line.topAnchor constraintEqualToAnchor:row.topAnchor],
      [line.bottomAnchor constraintEqualToAnchor:row.bottomAnchor],
      [line.leadingAnchor constraintEqualToAnchor:row.leadingAnchor],
      [line.trailingAnchor constraintEqualToAnchor:row.trailingAnchor],
    ]];

    NSStackView *inset = [NSStackView new];
    inset.edgeInsets = NSEdgeInsetsMake(0, 8, 0, 8);
    inset.translatesAutoresizingMaskIntoConstraints = NO;
    [inset addArrangedSubview:row];
    [row.widthAnchor constraintEqualToConstant:kSidebarWidth - 16].active = YES;
    [box addArrangedSubview:inset];
  }
  return box;
}

#pragma mark - rows

// A key and value line. The value may carry a tertiary suffix, a second line of
// explanation, a switch or the appearance control.
- (NSView *)kvRow:(NSDictionary *)row {
  BOOL tall = [row[@"hint"] length] > 0;
  NSStackView *line = [NSStackView new];
  line.orientation = NSUserInterfaceLayoutOrientationHorizontal;
  line.alignment = tall ? NSLayoutAttributeTop : NSLayoutAttributeCenterY;
  line.spacing = 0;
  BOOL half = [row[@"halfPad"] boolValue];
  CGFloat pad = half ? kCardPad / 2 : (tall ? 11 : 9);
  line.edgeInsets = NSEdgeInsetsMake(pad, 14, pad, 14);
  if (half && !tall) {
    [line.heightAnchor constraintEqualToConstant:kCardPad + kCardLine].active = YES;
  }

  NSTextField *key = [NSTextField labelWithString:row[@"text"] ?: @""];
  key.font = [NSFont systemFontOfSize:12];
  key.textColor = [NSColor secondaryLabelColor];
  [key.widthAnchor constraintEqualToConstant:kKeyWidth].active = YES;
  if (half) hug(key);
  [line addArrangedSubview:key];

  // The value column: the value itself, a tertiary suffix on the same line, and
  // an explanation under it.
  NSStackView *value = [NSStackView new];
  value.orientation = NSUserInterfaceLayoutOrientationVertical;
  value.alignment = NSLayoutAttributeLeading;
  value.spacing = 3;

  NSStackView *first = [NSStackView new];
  first.orientation = NSUserInterfaceLayoutOrientationHorizontal;
  first.alignment = NSLayoutAttributeFirstBaseline;
  first.spacing = 0;

  NSString *link = row[@"link"];
  NSString *detail = row[@"detail"] ?: @"";
  BOOL mono = [row[@"mono"] boolValue];
  if (link.length) {
    NSButton *l = [NSButton buttonWithTitle:row[@"linkText"] ?: link
                                     target:self action:@selector(clicked:)];
    l.bezelStyle = NSBezelStyleInline;
    l.bordered = NO;
    l.contentTintColor = [NSColor linkColor];
    l.font = [NSFont systemFontOfSize:12];
    [l.cell setHighlightsBy:NSContentsCellMask];
    l.tag = [self claim:link];
    [first addArrangedSubview:l];
  } else if (detail.length) {
    NSTextField *v = [NSTextField labelWithString:detail];
    v.font = mono ? [NSFont monospacedSystemFontOfSize:11.5 weight:NSFontWeightRegular]
                  : [NSFont systemFontOfSize:12];
    // A value that does not fit is an identifier: an image reference or a path.
    // Both ends carry meaning, so the middle is dropped.
    v.lineBreakMode = NSLineBreakByTruncatingMiddle;
    [v setContentCompressionResistancePriority:NSLayoutPriorityDefaultLow
                                forOrientation:NSLayoutConstraintOrientationHorizontal];
    if (half) hug(v);
    [first addArrangedSubview:v];
  }
  NSString *faint = row[@"faint"];
  if (faint.length) {
    NSTextField *f = [NSTextField labelWithString:[@" · " stringByAppendingString:faint]];
    f.font = [NSFont systemFontOfSize:12];
    f.textColor = [NSColor tertiaryLabelColor];
    f.lineBreakMode = NSLineBreakByTruncatingTail;
    [f setContentCompressionResistancePriority:NSLayoutPriorityDefaultLow - 1
                                forOrientation:NSLayoutConstraintOrientationHorizontal];
    [first addArrangedSubview:f];
  }
  [value addArrangedSubview:first];

  NSString *hint = row[@"hint"];
  if (hint.length) {
    NSTextField *h = [NSTextField labelWithString:hint];
    h.font = [NSFont systemFontOfSize:12];
    h.textColor = [NSColor tertiaryLabelColor];
    h.lineBreakMode = NSLineBreakByTruncatingTail;
    [h setContentCompressionResistancePriority:NSLayoutPriorityDefaultLow
                                forOrientation:NSLayoutConstraintOrientationHorizontal];
    [value addArrangedSubview:h];
  }
  [line addArrangedSubview:value];

  NSView *spacer = [NSView new];
  [spacer setContentHuggingPriority:1 forOrientation:NSLayoutConstraintOrientationHorizontal];
  [line addArrangedSubview:spacer];

  // The key and the value run together, so the row's spacing is zero. The
  // controls at the trailing edge are set apart one by one.
  NSMutableArray<NSView *> *controls = [NSMutableArray array];
  // The appearance control appears twice: in the title bar and here. Both write
  // the same stored choice, so this one is the same control.
  if ([row[@"appearance"] boolValue]) {
    [controls addObject:[self appearanceControl]];
  }
  NSDictionary *seg = row[@"segment"];
  if (seg) {
    NSSegmentedControl *c =
        [NSSegmentedControl segmentedControlWithLabels:seg[@"labels"]
                                          trackingMode:NSSegmentSwitchTrackingSelectOne
                                                target:self
                                                action:@selector(segmentChanged:)];
    c.controlSize = NSControlSizeSmall;
    c.font = [NSFont systemFontOfSize:11];
    c.selectedSegment = [seg[@"selected"] integerValue];
    c.tag = [self claim:seg[@"id"]];
    [controls addObject:c];
  }
  NSString *toggle = row[@"toggle"];
  if (toggle.length) {
    NSSwitch *sw = [NSSwitch new];
    sw.state = [row[@"on"] boolValue] ? NSControlStateValueOn : NSControlStateValueOff;
    sw.enabled = ![row[@"disabled"] boolValue];
    sw.target = self;
    sw.action = @selector(switched:);
    sw.tag = [self claim:toggle];
    sw.controlSize = NSControlSizeSmall;
    [controls addObject:sw];
  }
  for (NSDictionary *b in row[@"buttons"]) [controls addObject:[self buttonFor:b]];

  NSView *before = spacer;
  for (NSView *c in controls) {
    [line addArrangedSubview:c];
    [line setCustomSpacing:kControlGap afterView:before];
    before = c;
  }

  // A stack aligned to the top does not grow to hold a column taller than the
  // rest of the row, so the padding under the value is stated here. Without it
  // an explanation on a second line is drawn past the card's edge.
  [line.bottomAnchor constraintGreaterThanOrEqualToAnchor:value.bottomAnchor
                                                 constant:tall ? 11 : 9].active = YES;
  return line;
}

// A row that names a selection opens it when clicked. The buttons inside it
// handle their own clicks, so they are not affected.
- (NSView *)clickableRow:(NSDictionary *)row {
  NSView *line = [self rowFor:row];
  NSString *rowID = row[@"id"];
  if (!rowID.length) return line;

  ClickableRow *hit = [ClickableRow new];
  hit.owner = self;
  hit.action = [self claim:rowID];
  line.translatesAutoresizingMaskIntoConstraints = NO;
  [hit addSubview:line];
  [NSLayoutConstraint activateConstraints:@[
    [line.topAnchor constraintEqualToAnchor:hit.topAnchor],
    [line.bottomAnchor constraintEqualToAnchor:hit.bottomAnchor],
    [line.leadingAnchor constraintEqualToAnchor:hit.leadingAnchor],
    [line.trailingAnchor constraintEqualToAnchor:hit.trailingAnchor],
  ]];
  return hit;
}

// titleRow names what the card is about and carries the actions on that
// subject. A card that stands for one thing states it inside the card, not in
// the group header above it, which names a group of cards.
- (NSView *)titleRow:(NSDictionary *)row {
  NSStackView *line = [NSStackView new];
  line.orientation = NSUserInterfaceLayoutOrientationHorizontal;
  line.alignment = NSLayoutAttributeCenterY;
  line.spacing = 8;
  BOOL half = [row[@"halfPad"] boolValue];
  CGFloat pad = half ? kCardPad / 2 : 12;
  line.edgeInsets = NSEdgeInsetsMake(pad, 14, pad, 14);

  NSTextField *t = [NSTextField labelWithString:row[@"text"] ?: @""];
  t.font = [NSFont systemFontOfSize:15 weight:NSFontWeightSemibold];
  t.lineBreakMode = NSLineBreakByTruncatingTail;
  if (half) hug(t);
  [line addArrangedSubview:t];

  NSView *spacer = [NSView new];
  [spacer setContentHuggingPriority:1 forOrientation:NSLayoutConstraintOrientationHorizontal];
  [line addArrangedSubview:spacer];

  for (NSDictionary *b in row[@"buttons"]) [line addArrangedSubview:[self buttonFor:b]];
  return line;
}

// labelRow names the rows under it inside a card.
- (NSView *)labelRow:(NSDictionary *)row {
  NSStackView *line = [NSStackView new];
  line.orientation = NSUserInterfaceLayoutOrientationHorizontal;
  line.alignment = NSLayoutAttributeCenterY;
  BOOL half = [row[@"halfPad"] boolValue];
  CGFloat pad = half ? kCardPad / 2 : 10;
  line.edgeInsets = NSEdgeInsetsMake(pad, 14, pad, 14);

  NSTextField *t = [NSTextField labelWithString:row[@"text"] ?: @""];
  t.font = [NSFont systemFontOfSize:11];
  t.textColor = [NSColor secondaryLabelColor];
  if (half) hug(t);
  [line addArrangedSubview:t];
  return line;
}

// hug holds a label's box to the line its type draws, so the space between two
// rows is the padding and nothing more.
static void hug(NSTextField *t) {
  CGFloat line = ceil(t.font.ascender - t.font.descender);
  [t.heightAnchor constraintEqualToConstant:line].active = YES;
}

- (NSView *)rowFor:(NSDictionary *)row {
  if ([row[@"kind"] isEqualToString:@"kv"]) return [self kvRow:row];
  if ([row[@"kind"] isEqualToString:@"title"]) return [self titleRow:row];
  if ([row[@"kind"] isEqualToString:@"label"]) return [self labelRow:row];

  NSStackView *line = [NSStackView new];
  line.orientation = NSUserInterfaceLayoutOrientationHorizontal;
  line.alignment = NSLayoutAttributeCenterY;
  line.spacing = 12;
  BOOL half = [row[@"halfPad"] boolValue];
  CGFloat pad = half ? kCardPad / 2 : 10;
  line.edgeInsets = NSEdgeInsetsMake(pad, 14, pad, half ? 14 : 12);

  [line addArrangedSubview:[DotView dot:row[@"dot"] ?: @""]];

  NSTextField *name = [NSTextField labelWithString:row[@"text"] ?: @""];
  name.font = [NSFont systemFontOfSize:13];
  if (half) hug(name);
  BOOL wide = [row[@"wide"] boolValue];
  if (wide) {
    // A certificate name can be longer than the column. It is what tells the
    // rows apart, so it takes the width the row does not otherwise need, and
    // what does not fit is dropped from the middle: the beginning and the end
    // both distinguish one name from another.
    name.lineBreakMode = NSLineBreakByTruncatingMiddle;
    [name.widthAnchor constraintGreaterThanOrEqualToConstant:kNameWideWidth].active = YES;
    [name setContentHuggingPriority:1 forOrientation:NSLayoutConstraintOrientationHorizontal];
    // The name takes the width the row does not need. What the row does need is
    // the short phrase beside it, so the name is what gives way first.
    [name setContentCompressionResistancePriority:NSLayoutPriorityDefaultLow
                                   forOrientation:NSLayoutConstraintOrientationHorizontal];
  } else {
    name.lineBreakMode = NSLineBreakByTruncatingTail;
    [name.widthAnchor constraintEqualToConstant:kNameWidth].active = YES;
  }
  [line addArrangedSubview:name];

  NSString *chip = row[@"chip"];
  if (chip.length) [line addArrangedSubview:[self chip:chip]];

  // A row of dots reports the state of each service in a project.
  NSArray *dots = row[@"dots"];
  if (dots.count) {
    NSStackView *run = [NSStackView new];
    run.orientation = NSUserInterfaceLayoutOrientationHorizontal;
    run.spacing = 4;
    for (NSString *d in dots) [run addArrangedSubview:[DotView dot:d]];
    [line addArrangedSubview:run];
  }

  // The address column takes the free width. It holds a link when the row has a
  // URL and muted text otherwise.
  NSString *link = row[@"link"];
  NSString *linkText = row[@"linkText"] ?: link;
  NSView *reach = nil;
  if (link.length) {
    NSButton *l = [NSButton buttonWithTitle:linkText target:self action:@selector(clicked:)];
    l.bezelStyle = NSBezelStyleInline;
    l.bordered = NO;
    l.contentTintColor = [NSColor linkColor];
    l.font = [NSFont systemFontOfSize:12];
    l.alignment = NSTextAlignmentLeft;
    [l.cell setHighlightsBy:NSContentsCellMask];
    l.tag = [self claim:link];
    reach = l;
  } else {
    NSTextField *t = [NSTextField labelWithString:linkText ?: @""];
    t.font = [NSFont systemFontOfSize:12];
    t.textColor = [NSColor secondaryLabelColor];
    t.lineBreakMode = NSLineBreakByTruncatingHead;
    reach = t;
  }
  // On a row whose name takes the free width, the address column keeps to its
  // own text instead of absorbing it.
  [reach setContentHuggingPriority:wide ? NSLayoutPriorityDefaultLow : 1
                    forOrientation:NSLayoutConstraintOrientationHorizontal];
  [reach setContentCompressionResistancePriority:wide ? NSLayoutPriorityDefaultHigh
                                                      : NSLayoutPriorityDefaultLow
                                  forOrientation:NSLayoutConstraintOrientationHorizontal];
  [line addArrangedSubview:reach];

  NSTextField *detail = [NSTextField labelWithString:row[@"detail"] ?: @""];
  detail.textColor = [NSColor secondaryLabelColor];
  detail.font = [NSFont systemFontOfSize:12];
  detail.alignment = NSTextAlignmentRight;
  detail.lineBreakMode = NSLineBreakByTruncatingHead;
  [detail setContentCompressionResistancePriority:NSLayoutPriorityDefaultHigh - 1
                                   forOrientation:NSLayoutConstraintOrientationHorizontal];
  [line addArrangedSubview:detail];

  for (NSDictionary *b in row[@"buttons"]) [line addArrangedSubview:[self buttonFor:b]];
  return line;
}

// logPane draws the tail of a container's output on the code background.
- (NSView *)logPane:(NSArray *)lines {
  CodePane *pane = [[CodePane alloc] initWithFrame:NSZeroRect];
  NSStackView *stack = [NSStackView new];
  stack.orientation = NSUserInterfaceLayoutOrientationVertical;
  stack.alignment = NSLayoutAttributeLeading;
  stack.spacing = 3;
  stack.edgeInsets = NSEdgeInsetsMake(11, kCodePad, 12, kCodePad);
  stack.translatesAutoresizingMaskIntoConstraints = NO;
  if (!lines.count) {
    lines = @[label(@"noOutput", @"(no output yet)")];
  }
  for (NSString *l in lines) {
    NSTextField *t = [NSTextField labelWithString:l];
    t.font = [NSFont monospacedSystemFontOfSize:11 weight:NSFontWeightRegular];
    t.lineBreakMode = NSLineBreakByTruncatingTail;
    [t setContentCompressionResistancePriority:NSLayoutPriorityDefaultLow
                                forOrientation:NSLayoutConstraintOrientationHorizontal];
    [stack addArrangedSubview:t];
  }
  [pane addSubview:stack];
  [NSLayoutConstraint activateConstraints:@[
    [stack.topAnchor constraintEqualToAnchor:pane.topAnchor],
    [stack.leadingAnchor constraintEqualToAnchor:pane.leadingAnchor],
    [stack.trailingAnchor constraintEqualToAnchor:pane.trailingAnchor],
    [stack.bottomAnchor constraintEqualToAnchor:pane.bottomAnchor],
  ]];
  // A line is as wide as the pane's content, not the pane, so the padding on
  // both sides is left alone.
  for (NSView *v in stack.arrangedSubviews) {
    [v.widthAnchor constraintEqualToAnchor:stack.widthAnchor
                                constant:-2 * kCodePad].active = YES;
  }
  return pane;
}

- (NSView *)logFoot:(NSDictionary *)section {
  NSStackView *line = [NSStackView new];
  line.orientation = NSUserInterfaceLayoutOrientationHorizontal;
  line.alignment = NSLayoutAttributeCenterY;
  line.spacing = 10;
  line.edgeInsets = NSEdgeInsetsMake(8, 14, 8, 12);
  NSTextField *t = [NSTextField labelWithString:section[@"logNote"] ?: @""];
  t.font = [NSFont systemFontOfSize:12];
  t.textColor = [NSColor secondaryLabelColor];
  [line addArrangedSubview:t];
  NSView *spacer = [NSView new];
  [spacer setContentHuggingPriority:1 forOrientation:NSLayoutConstraintOrientationHorizontal];
  [line addArrangedSubview:spacer];
  for (NSDictionary *b in section[@"logButtons"]) [line addArrangedSubview:[self buttonFor:b]];
  return line;
}

- (NSView *)sectionFor:(NSDictionary *)section {
  NSStackView *box = [NSStackView new];
  box.orientation = NSUserInterfaceLayoutOrientationVertical;
  box.alignment = NSLayoutAttributeLeading;
  box.spacing = 7;

  NSString *headerText = section[@"header"];
  NSStackView *head = nil;
  if (headerText.length || [section[@"buttons"] count]) {
    head = [NSStackView new];
    head.orientation = NSUserInterfaceLayoutOrientationHorizontal;
    head.alignment = NSLayoutAttributeCenterY;
    head.spacing = 8;
    head.edgeInsets = NSEdgeInsetsMake(0, 4, 0, 2);
    if (headerText.length) {
      NSTextField *t = [NSTextField labelWithString:headerText];
      t.font = [NSFont systemFontOfSize:11 weight:NSFontWeightSemibold];
      t.textColor = [NSColor secondaryLabelColor];
      [head addArrangedSubview:t];
    }
    NSString *note = section[@"note"];
    if (note.length && ([section[@"rows"] count] > 0 || section[@"log"])) {
      NSTextField *n = [NSTextField labelWithString:note];
      n.font = [NSFont systemFontOfSize:11];
      n.textColor = [NSColor tertiaryLabelColor];
      n.lineBreakMode = NSLineBreakByTruncatingTail;
      [n setContentCompressionResistancePriority:NSLayoutPriorityDefaultLow
                                  forOrientation:NSLayoutConstraintOrientationHorizontal];
      [head addArrangedSubview:n];
    }
    NSView *spacer = [NSView new];
    [spacer setContentHuggingPriority:1 forOrientation:NSLayoutConstraintOrientationHorizontal];
    [head addArrangedSubview:spacer];
    for (NSDictionary *b in section[@"buttons"]) [head addArrangedSubview:[self buttonFor:b]];
    [box addArrangedSubview:head];
  }

  Card *card = [[Card alloc] initWithFrame:NSZeroRect];
  NSStackView *inner = [NSStackView new];
  inner.orientation = NSUserInterfaceLayoutOrientationVertical;
  inner.alignment = NSLayoutAttributeLeading;
  inner.spacing = 0;
  inner.translatesAutoresizingMaskIntoConstraints = NO;

  NSArray *rows = section[@"rows"];
  NSString *note = section[@"note"];
  if (!rows.count && !section[@"log"] && note.length) {
    NSTextField *n = [NSTextField wrappingLabelWithString:note];
    n.textColor = [NSColor secondaryLabelColor];
    n.font = [NSFont systemFontOfSize:12];
    NSStackView *wrap = [NSStackView new];
    wrap.orientation = NSUserInterfaceLayoutOrientationVertical;
    wrap.alignment = NSLayoutAttributeLeading;
    wrap.edgeInsets = NSEdgeInsetsMake(11, 14, 11, 14);
    [wrap addArrangedSubview:n];
    [inner addArrangedSubview:wrap];
  }
  // A card whose rows name its parts draws a rule only where a part begins,
  // which is a label row. A card that is a plain list draws one between every
  // row, because there the rows are the items.
  BOOL named = NO;
  for (NSDictionary *r in rows) {
    NSString *kind = r[@"kind"];
    if ([kind isEqualToString:@"title"] || [kind isEqualToString:@"label"]) named = YES;
  }
  if (named) inner.edgeInsets = NSEdgeInsetsMake(kCardPad / 2, 0, kCardPad / 2, 0);
  NSView *previous = nil;
  for (NSUInteger i = 0; i < rows.count; i++) {
    if (i > 0) {
      BOOL rule = named ? [rows[i][@"kind"] isEqualToString:@"label"] : YES;
      if (rule) {
        NSView *h = [self hairline];
        [inner addArrangedSubview:h];
        if (named) {
          [NSLayoutConstraint activateConstraints:@[
            [h.leadingAnchor constraintEqualToAnchor:inner.leadingAnchor constant:14],
            [h.trailingAnchor constraintEqualToAnchor:inner.trailingAnchor constant:-14],
          ]];
          // A rule separates one part of a card from the next, so the boundary
          // it marks takes a padding on each side of it, where a boundary with
          // no rule takes one padding in all.
          [inner setCustomSpacing:kCardPad / 2 afterView:previous];
          [inner setCustomSpacing:kCardPad / 2 afterView:h];
        }
        previous = h;
      }
    }
    NSDictionary *r = rows[i];
    if (named) {
      NSMutableDictionary *m = [r mutableCopy];
      m[@"halfPad"] = @YES;
      r = m;
    }
    NSView *v = [self clickableRow:r];
    [inner addArrangedSubview:v];
    previous = v;
  }

  // A log section carries the tail of a container's output and a strip naming
  // what the pane is showing.
  NSArray *log = section[@"log"];
  if (log) {
    if (rows.count) [inner addArrangedSubview:[self hairline]];
    [inner addArrangedSubview:[self logPane:log]];
    [inner addArrangedSubview:[self hairline]];
    [inner addArrangedSubview:[self logFoot:section]];
  }

  [card addSubview:inner];
  [NSLayoutConstraint activateConstraints:@[
    [inner.topAnchor constraintEqualToAnchor:card.topAnchor],
    [inner.leadingAnchor constraintEqualToAnchor:card.leadingAnchor],
    [inner.trailingAnchor constraintEqualToAnchor:card.trailingAnchor],
    [inner.bottomAnchor constraintEqualToAnchor:card.bottomAnchor],
  ]];
  [box addArrangedSubview:card];

  for (NSView *v in inner.arrangedSubviews) {
    [v.widthAnchor constraintEqualToAnchor:inner.widthAnchor].active = YES;
  }
  [card.widthAnchor constraintEqualToAnchor:box.widthAnchor].active = YES;
  if (head) [head.widthAnchor constraintEqualToAnchor:box.widthAnchor].active = YES;
  return box;
}

#pragma mark - render

- (NSView *)verdictFor:(NSDictionary *)v {
  NSStackView *line = [NSStackView new];
  line.orientation = NSUserInterfaceLayoutOrientationHorizontal;
  line.alignment = NSLayoutAttributeCenterY;
  // The beacon's halo is drawn inside its view, so the view is wider than the
  // dot. The leading inset and the spacing take that width back, which leaves
  // the dot where a plain dot would sit.
  line.spacing = 10 - kBeaconHalo;
  line.edgeInsets = NSEdgeInsetsMake(0, -kBeaconHalo, 0, 0);

  [line addArrangedSubview:[DotView beacon:v[@"dot"] ?: @"on"]];

  NSStackView *text = [NSStackView new];
  text.orientation = NSUserInterfaceLayoutOrientationVertical;
  text.alignment = NSLayoutAttributeLeading;
  text.spacing = 2;
  NSTextField *head = [NSTextField labelWithString:v[@"headline"] ?: @""];
  head.font = [NSFont systemFontOfSize:15 weight:NSFontWeightSemibold];
  [text addArrangedSubview:head];
  NSString *sub = v[@"subline"];
  if (sub.length) {
    NSTextField *s = [NSTextField labelWithString:sub];
    s.font = [NSFont systemFontOfSize:12];
    s.textColor = [NSColor secondaryLabelColor];
    [text addArrangedSubview:s];
  }
  [line addArrangedSubview:text];
  return line;
}

// A banner states one condition of the machine and carries the action that
// answers it. "bad" is a fault; anything else is a warning.
- (NSView *)bannerFor:(NSDictionary *)b {
  BOOL bad = [b[@"kind"] isEqualToString:@"bad"];
  TintedCard *card = [[TintedCard alloc] initWithFrame:NSZeroRect];
  card.fill = bad ? dynamicColor(hex(0xFFEFED), hex(0x2A1D1C))
                  : dynamicColor(hex(0xFFF6E0), hex(0x24211A));
  card.edge = bad ? dynamicColor(hex(0xF0C7C2), hex(0x5A2B27))
                  : dynamicColor(hex(0xEBD9A8), hex(0x4A3D1E));
  NSColor *titleColor = bad ? dynamicColor(hex(0x7A2A22), hex(0xF3D3D0))
                            : dynamicColor(hex(0x6B4E11), hex(0xE9DFC2));
  NSColor *textColor = bad ? dynamicColor(hex(0x8C3A31), hex(0xD2ABA7))
                           : dynamicColor(hex(0x7C6329), hex(0xC9BE9E));

  NSStackView *line = [NSStackView new];
  line.orientation = NSUserInterfaceLayoutOrientationHorizontal;
  line.alignment = NSLayoutAttributeCenterY;
  line.spacing = 14;
  line.edgeInsets = NSEdgeInsetsMake(14, 16, 14, 16);
  line.translatesAutoresizingMaskIntoConstraints = NO;

  NSStackView *text = [NSStackView new];
  text.orientation = NSUserInterfaceLayoutOrientationVertical;
  text.alignment = NSLayoutAttributeLeading;
  text.spacing = 2;
  NSTextField *title = [NSTextField labelWithString:b[@"title"] ?: @""];
  title.font = [NSFont systemFontOfSize:13 weight:NSFontWeightSemibold];
  title.textColor = titleColor;
  [text addArrangedSubview:title];
  NSTextField *body = [NSTextField wrappingLabelWithString:b[@"text"] ?: @""];
  body.font = [NSFont systemFontOfSize:12];
  body.textColor = textColor;
  [text addArrangedSubview:body];
  [line addArrangedSubview:text];

  NSView *spacer = [NSView new];
  [spacer setContentHuggingPriority:1 forOrientation:NSLayoutConstraintOrientationHorizontal];
  [line addArrangedSubview:spacer];
  for (NSDictionary *button in b[@"buttons"]) [line addArrangedSubview:[self buttonFor:button]];
  [line.bottomAnchor constraintGreaterThanOrEqualToAnchor:text.bottomAnchor constant:14].active = YES;

  [card addSubview:line];
  [NSLayoutConstraint activateConstraints:@[
    [line.topAnchor constraintEqualToAnchor:card.topAnchor],
    [line.leadingAnchor constraintEqualToAnchor:card.leadingAnchor],
    [line.trailingAnchor constraintEqualToAnchor:card.trailingAnchor],
    [line.bottomAnchor constraintEqualToAnchor:card.bottomAnchor],
  ]];
  return card;
}

- (void)render:(NSString *)json {
  [self build];
  NSError *err = nil;
  NSDictionary *model = [NSJSONSerialization
      JSONObjectWithData:[json dataUsingEncoding:NSUTF8StringEncoding] options:0 error:&err];
  if (!model) return;

  [self.actionIds removeAllObjects];
  self.bodyAppearance = nil;
  for (NSStackView *stack in @[self.sidebar, self.content, self.headerBar]) {
    for (NSView *v in [stack.arrangedSubviews copy]) {
      [stack removeArrangedSubview:v];
      [v removeFromSuperview];
    }
  }

  for (NSDictionary *group in model[@"sidebar"]) {
    NSView *v = [self sidebarGroup:group];
    [self.sidebar addArrangedSubview:v];
    [v.widthAnchor constraintEqualToAnchor:self.sidebar.widthAnchor].active = YES;
  }

  NSDictionary *header = model[@"header"];
  self.headerTitle.stringValue = header[@"title"] ?: @"";
  [self.headerBar addArrangedSubview:self.headerTitle];
  NSString *sub = header[@"subtitle"];
  self.headerSubtitle.stringValue = sub ?: @"";
  if (sub.length) [self.headerBar addArrangedSubview:self.headerSubtitle];
  NSView *hspacer = [NSView new];
  [hspacer setContentHuggingPriority:1 forOrientation:NSLayoutConstraintOrientationHorizontal];
  [self.headerBar addArrangedSubview:hspacer];
  for (NSDictionary *b in header[@"buttons"]) [self.headerBar addArrangedSubview:[self buttonFor:b]];

  NSString *message = model[@"message"];
  if (message.length) {
    BOOL bad = [model[@"messageKind"] isEqualToString:@"error"];
    Card *card = [[Card alloc] initWithFrame:NSZeroRect];
    NSTextField *t = [NSTextField wrappingLabelWithString:message];
    t.font = [NSFont systemFontOfSize:12];
    t.textColor = bad ? [NSColor systemRedColor] : [NSColor secondaryLabelColor];
    t.translatesAutoresizingMaskIntoConstraints = NO;
    [card addSubview:t];
    [NSLayoutConstraint activateConstraints:@[
      [t.topAnchor constraintEqualToAnchor:card.topAnchor constant:11],
      [t.bottomAnchor constraintEqualToAnchor:card.bottomAnchor constant:-11],
      [t.leadingAnchor constraintEqualToAnchor:card.leadingAnchor constant:14],
      [t.trailingAnchor constraintEqualToAnchor:card.trailingAnchor constant:-14],
    ]];
    [self.content addArrangedSubview:card];
    [card.widthAnchor constraintEqualToAnchor:self.content.widthAnchor].active = YES;
  }

  if (model[@"verdict"]) {
    NSView *v = [self verdictFor:model[@"verdict"]];
    [self.content addArrangedSubview:v];
  }
  if (model[@"banner"]) {
    NSView *v = [self bannerFor:model[@"banner"]];
    [self.content addArrangedSubview:v];
    [v.widthAnchor constraintEqualToAnchor:self.content.widthAnchor].active = YES;
  }
  for (NSDictionary *section in model[@"sections"]) {
    NSView *v = [self sectionFor:section];
    [self.content addArrangedSubview:v];
    [v.widthAnchor constraintEqualToAnchor:self.content.widthAnchor].active = YES;
  }
}

- (void)showWith:(NSString *)json {
  [self render:json];
  [self.window makeKeyAndOrderFront:nil];
  // An LSUIElement process is not in the activation order, so the window is
  // activated explicitly.
  [NSApp activateIgnoringOtherApps:YES];
}

- (BOOL)visible { return self.window != nil && self.window.isVisible; }
@end

#pragma mark - log window

@interface LogWindow : NSObject
@property(strong) NSWindow *window;
@property(strong) NSTextView *text;
@end

@implementation LogWindow
+ (instancetype)shared {
  static LogWindow *w;
  static dispatch_once_t once;
  dispatch_once(&once, ^{ w = [LogWindow new]; });
  return w;
}

- (void)showTitle:(NSString *)title text:(NSString *)body atEnd:(BOOL)atEnd {
  if (!self.window) {
    self.window = [[NSWindow alloc]
        initWithContentRect:NSMakeRect(0, 0, 760, 460)
                  styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable |
                            NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable
                    backing:NSBackingStoreBuffered
                      defer:NO];
    self.window.releasedWhenClosed = NO;
    [self.window center];

    NSScrollView *scroll = [[NSScrollView alloc] initWithFrame:self.window.contentView.bounds];
    scroll.hasVerticalScroller = YES;
    scroll.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;

    self.text = [[NSTextView alloc] initWithFrame:scroll.bounds];
    self.text.editable = NO;
    self.text.richText = NO;
    self.text.font = [NSFont monospacedSystemFontOfSize:11 weight:NSFontWeightRegular];
    self.text.autoresizingMask = NSViewWidthSizable;
    self.text.textContainerInset = NSMakeSize(10, 10);
    scroll.documentView = self.text;
    self.window.contentView = scroll;
  }
  self.window.title = title;
  self.text.string = body;
  [self.text scrollRangeToVisible:atEnd ? NSMakeRange(self.text.string.length, 0)
                                        : NSMakeRange(0, 0)];
  [self.window makeKeyAndOrderFront:nil];
  [NSApp activateIgnoringOtherApps:YES];
}
@end

#pragma mark - dialogs

void ui_prompt(const char *actionID, const char *title, const char *message,
               const char *placeholder, const char *initial) {
  NSString *aid = [NSString stringWithUTF8String:actionID ?: ""];
  NSString *t = [NSString stringWithUTF8String:title ?: ""];
  NSString *m = [NSString stringWithUTF8String:message ?: ""];
  NSString *ph = [NSString stringWithUTF8String:placeholder ?: ""];
  NSString *init = [NSString stringWithUTF8String:initial ?: ""];

  dispatch_async(dispatch_get_main_queue(), ^{
    NSAlert *alert = [NSAlert new];
    alert.messageText = t;
    alert.informativeText = m;
    [alert addButtonWithTitle:label(@"ok", @"OK")];
    [alert addButtonWithTitle:label(@"cancel", @"Cancel")];

    NSTextField *field = [[NSTextField alloc] initWithFrame:NSMakeRect(0, 0, 300, 24)];
    field.placeholderString = ph;
    field.stringValue = init;
    alert.accessoryView = field;
    [alert.window setInitialFirstResponder:field];

    if ([alert runModal] == NSAlertFirstButtonReturn) {
      NSString *v = [field.stringValue stringByTrimmingCharactersInSet:
                        [NSCharacterSet whitespaceAndNewlineCharacterSet]];
      if (v.length) goUIPrompt((char *)[aid UTF8String], (char *)[v UTF8String]);
    }
  });
}

void ui_confirm(const char *actionID, const char *title, const char *message,
                const char *okTitle, int destructive) {
  NSString *aid = [NSString stringWithUTF8String:actionID ?: ""];
  NSString *t = [NSString stringWithUTF8String:title ?: ""];
  NSString *m = [NSString stringWithUTF8String:message ?: ""];
  NSString *ok = [NSString stringWithUTF8String:okTitle ?: "OK"];

  dispatch_async(dispatch_get_main_queue(), ^{
    NSAlert *alert = [NSAlert new];
    alert.messageText = t;
    alert.informativeText = m;
    alert.alertStyle = destructive ? NSAlertStyleCritical : NSAlertStyleWarning;
    NSButton *okButton = [alert addButtonWithTitle:ok];
    [alert addButtonWithTitle:label(@"cancel", @"Cancel")];
    if (destructive) okButton.hasDestructiveAction = YES;
    if ([alert runModal] == NSAlertFirstButtonReturn) {
      goUIAction((char *)[aid UTF8String]);
    }
  });
}

void ui_pick(const char *actionID, const char *title, const char *prompt) {
  NSString *aid = [NSString stringWithUTF8String:actionID ?: ""];
  NSString *t = [NSString stringWithUTF8String:title ?: ""];
  NSString *p = [NSString stringWithUTF8String:prompt ?: ""];
  dispatch_async(dispatch_get_main_queue(), ^{
    NSOpenPanel *panel = [NSOpenPanel openPanel];
    panel.message = t;
    panel.prompt = p;
    panel.canChooseDirectories = YES;
    panel.canChooseFiles = YES;
    panel.allowsMultipleSelection = NO;
    [NSApp activateIgnoringOtherApps:YES];
    if ([panel runModal] == NSModalResponseOK && panel.URL) {
      const char *path = panel.URL.path.UTF8String;
      goUIPrompt((char *)[aid UTF8String], (char *)path);
    }
  });
}

void ui_labels(const char *json) {
  NSData *data = [[NSString stringWithUTF8String:json ?: "{}"]
      dataUsingEncoding:NSUTF8StringEncoding];
  NSDictionary *d = [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
  dispatch_async(dispatch_get_main_queue(), ^{
    if ([d isKindOfClass:[NSDictionary class]]) gLabels = d;
  });
}

void ui_language(char *out, int n) {
  NSString *tag = [[NSLocale preferredLanguages] firstObject] ?: @"en";
  strlcpy(out, tag.UTF8String, (size_t)n);
}

void ui_flag(const char *key, int *out) {
  NSString *k = [NSString stringWithUTF8String:key ?: ""];
  *out = [[NSUserDefaults standardUserDefaults] boolForKey:k] ? 1 : 0;
}

void ui_set_flag(const char *key, int value) {
  NSString *k = [NSString stringWithUTF8String:key ?: ""];
  [[NSUserDefaults standardUserDefaults] setBool:(value != 0) forKey:k];
}

void ui_text(const char *key, char *out, int n) {
  NSString *k = [NSString stringWithUTF8String:key ?: ""];
  NSString *v = [[NSUserDefaults standardUserDefaults] stringForKey:k] ?: @"";
  strlcpy(out, v.UTF8String, (size_t)n);
}

void ui_set_text(const char *key, const char *value) {
  NSString *k = [NSString stringWithUTF8String:key ?: ""];
  NSString *v = [NSString stringWithUTF8String:value ?: ""];
  [[NSUserDefaults standardUserDefaults] setObject:v forKey:k];
}

void ui_copy(const char *text) {
  NSString *s = [NSString stringWithUTF8String:text ?: ""];
  dispatch_async(dispatch_get_main_queue(), ^{
    NSPasteboard *pb = [NSPasteboard generalPasteboard];
    [pb clearContents];
    [pb setString:s forType:NSPasteboardTypeString];
  });
}

void ui_logs(const char *title, const char *text, int atEnd) {
  NSString *t = [NSString stringWithUTF8String:title ?: ""];
  NSString *b = [NSString stringWithUTF8String:text ?: ""];
  BOOL end = atEnd != 0;
  dispatch_async(dispatch_get_main_queue(), ^{
    [[LogWindow shared] showTitle:t text:b atEnd:end];
  });
}

void ui_is_visible(int *out) {
  __block int v = 0;
  dispatch_sync(dispatch_get_main_queue(), ^{ v = [[PanelController shared] visible] ? 1 : 0; });
  *out = v;
}

void ui_show(const char *json) {
  NSString *s = [NSString stringWithUTF8String:json ?: ""];
  dispatch_async(dispatch_get_main_queue(), ^{ [[PanelController shared] showWith:s]; });
}

void ui_update(const char *json) {
  NSString *s = [NSString stringWithUTF8String:json ?: ""];
  dispatch_async(dispatch_get_main_queue(), ^{
    PanelController *c = [PanelController shared];
    if ([c visible]) [c render:s];
  });
}

#pragma mark - sheet

// SheetController is the one sheet the window shows: a name to type and one
// choice to make. It is attached to the panel window, so the window stays on
// screen behind it and the machine state it describes stays visible.
@interface SheetController : NSObject <NSTextFieldDelegate>
@property(strong) NSWindow *sheet;
@property(strong) NSTextField *field;
@property(strong) Chip *preview;
@property(strong) NSSwitch *option;
@property(copy) NSString *actionID;
@property(copy) NSString *suffix;
@end

@implementation SheetController

+ (instancetype)shared {
  static SheetController *c;
  static dispatch_once_t once;
  dispatch_once(&once, ^{ c = [SheetController new]; });
  return c;
}

- (void)controlTextDidChange:(NSNotification *)note {
  [self updatePreview];
}

// The preview shows the name the sheet would produce, because a single-label
// wildcard is rejected by clients and the resulting name is what matters.
- (void)updatePreview {
  NSString *typed = [self.field.stringValue
      stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]];
  self.preview.label.stringValue = typed.length
      ? [NSString stringWithFormat:@"%@%@", self.suffix, typed]
      : @"";
  self.preview.hidden = typed.length == 0;
}

- (void)cancel:(id)sender {
  [self.sheet.sheetParent endSheet:self.sheet returnCode:NSModalResponseCancel];
  [self.sheet orderOut:nil];
  self.sheet = nil;
}

- (void)accept:(id)sender {
  NSString *value = [self.field.stringValue
      stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]];
  NSString *aid = self.actionID;
  BOOL on = self.option.state == NSControlStateValueOn;
  [self.sheet.sheetParent endSheet:self.sheet returnCode:NSModalResponseOK];
  [self.sheet orderOut:nil];
  self.sheet = nil;
  if (value.length) {
    goUISheet((char *)[aid UTF8String], (char *)[value UTF8String], on ? 1 : 0);
  }
}

- (void)present:(NSDictionary *)spec on:(NSWindow *)parent {
  if (self.sheet) return;
  self.actionID = spec[@"id"];
  self.suffix = spec[@"suffix"] ?: @"";

  NSWindow *w = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, 440, 10)
                                            styleMask:NSWindowStyleMaskTitled
                                              backing:NSBackingStoreBuffered
                                                defer:NO];
  NSStackView *box = [NSStackView new];
  box.orientation = NSUserInterfaceLayoutOrientationVertical;
  box.alignment = NSLayoutAttributeLeading;
  box.spacing = 14;
  box.edgeInsets = NSEdgeInsetsMake(20, 22, 16, 22);
  box.translatesAutoresizingMaskIntoConstraints = NO;

  NSTextField *title = [NSTextField labelWithString:spec[@"title"] ?: @""];
  title.font = [NSFont systemFontOfSize:14 weight:NSFontWeightSemibold];
  [box addArrangedSubview:title];

  NSTextField *message = [NSTextField wrappingLabelWithString:spec[@"message"] ?: @""];
  message.font = [NSFont systemFontOfSize:12];
  message.textColor = [NSColor secondaryLabelColor];
  [box addArrangedSubview:message];

  NSStackView *frow = [NSStackView new];
  frow.orientation = NSUserInterfaceLayoutOrientationHorizontal;
  frow.alignment = NSLayoutAttributeCenterY;
  frow.spacing = 10;
  NSTextField *flabel = [NSTextField labelWithString:spec[@"fieldLabel"] ?: @""];
  flabel.font = [NSFont systemFontOfSize:12];
  flabel.textColor = [NSColor secondaryLabelColor];
  [flabel.widthAnchor constraintEqualToConstant:66].active = YES;
  [frow addArrangedSubview:flabel];

  self.field = [NSTextField new];
  self.field.font = [NSFont systemFontOfSize:12];
  self.field.placeholderString = spec[@"placeholder"] ?: @"";
  self.field.delegate = self;
  [frow addArrangedSubview:self.field];

  self.preview = [[Chip alloc] initWithText:@""];
  self.preview.hidden = YES;
  [frow addArrangedSubview:self.preview];
  [box addArrangedSubview:frow];

  NSString *optionLabel = spec[@"optionLabel"];
  if (optionLabel.length) {
    NSStackView *orow = [NSStackView new];
    orow.orientation = NSUserInterfaceLayoutOrientationHorizontal;
    orow.alignment = NSLayoutAttributeCenterY;
    orow.spacing = 10;
    NSTextField *okey = [NSTextField labelWithString:label(@"then", @"Then")];
    okey.font = [NSFont systemFontOfSize:12];
    okey.textColor = [NSColor secondaryLabelColor];
    [okey.widthAnchor constraintEqualToConstant:66].active = YES;
    [orow addArrangedSubview:okey];
    NSTextField *otext = [NSTextField labelWithString:optionLabel];
    otext.font = [NSFont systemFontOfSize:12];
    [orow addArrangedSubview:otext];
    NSView *spacer = [NSView new];
    [spacer setContentHuggingPriority:1 forOrientation:NSLayoutConstraintOrientationHorizontal];
    [orow addArrangedSubview:spacer];
    self.option = [NSSwitch new];
    self.option.controlSize = NSControlSizeSmall;
    self.option.state = [spec[@"optionOn"] boolValue] ? NSControlStateValueOn : NSControlStateValueOff;
    [orow addArrangedSubview:self.option];
    [box addArrangedSubview:orow];
    [orow.widthAnchor constraintEqualToAnchor:box.widthAnchor constant:-44].active = YES;
  }

  NSStackView *acts = [NSStackView new];
  acts.orientation = NSUserInterfaceLayoutOrientationHorizontal;
  acts.alignment = NSLayoutAttributeCenterY;
  acts.spacing = 8;
  NSView *aspacer = [NSView new];
  [aspacer setContentHuggingPriority:1 forOrientation:NSLayoutConstraintOrientationHorizontal];
  [acts addArrangedSubview:aspacer];
  NSButton *cancel = [NSButton buttonWithTitle:label(@"cancel", @"Cancel")
                                        target:self action:@selector(cancel:)];
  cancel.keyEquivalent = @"\033";
  [acts addArrangedSubview:cancel];
  NSButton *ok = [NSButton buttonWithTitle:spec[@"acceptTitle"] ?: @"OK"
                                    target:self action:@selector(accept:)];
  ok.keyEquivalent = @"\r";
  ok.bezelColor = [NSColor controlAccentColor];
  ok.contentTintColor = [NSColor whiteColor];
  [acts addArrangedSubview:ok];
  [box addArrangedSubview:acts];

  NSView *content = [NSView new];
  [content addSubview:box];
  w.contentView = content;
  [NSLayoutConstraint activateConstraints:@[
    [box.topAnchor constraintEqualToAnchor:content.topAnchor],
    [box.leadingAnchor constraintEqualToAnchor:content.leadingAnchor],
    [box.trailingAnchor constraintEqualToAnchor:content.trailingAnchor],
    [box.bottomAnchor constraintEqualToAnchor:content.bottomAnchor],
    [content.widthAnchor constraintEqualToConstant:440],
    [message.widthAnchor constraintEqualToAnchor:box.widthAnchor constant:-44],
    [frow.widthAnchor constraintEqualToAnchor:box.widthAnchor constant:-44],
    [acts.widthAnchor constraintEqualToAnchor:box.widthAnchor constant:-44],
  ]];

  self.sheet = w;
  [self updatePreview];
  [parent beginSheet:w completionHandler:^(NSModalResponse r) {}];
  [w makeFirstResponder:self.field];
}
@end

void ui_sheet(const char *actionID, const char *title, const char *message,
              const char *fieldLabel, const char *placeholder, const char *suffix,
              const char *optionLabel, int optionOn, const char *acceptTitle) {
  NSDictionary *spec = @{
    @"id": [NSString stringWithUTF8String:actionID ?: ""],
    @"title": [NSString stringWithUTF8String:title ?: ""],
    @"message": [NSString stringWithUTF8String:message ?: ""],
    @"fieldLabel": [NSString stringWithUTF8String:fieldLabel ?: ""],
    @"placeholder": [NSString stringWithUTF8String:placeholder ?: ""],
    @"suffix": [NSString stringWithUTF8String:suffix ?: ""],
    @"optionLabel": [NSString stringWithUTF8String:optionLabel ?: ""],
    @"optionOn": @(optionOn != 0),
    @"acceptTitle": [NSString stringWithUTF8String:acceptTitle ?: "OK"],
  };
  dispatch_async(dispatch_get_main_queue(), ^{
    PanelController *c = [PanelController shared];
    if (![c visible]) return;
    [[SheetController shared] present:spec on:[c window]];
  });
}
