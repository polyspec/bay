#import <Cocoa/Cocoa.h>
#include "window.h"
#include "_cgo_export.h"

// The window is a source list: a fixed-width sidebar for navigation and a pane
// showing one subject. Colours are read from the system palette, so light and
// dark use the same code.
static const CGFloat kSidebarWidth = 216;
static const CGFloat kNameWidth = 104;
static const CGFloat kReachWidth = 260;

static NSColor *dotColor(NSString *state) {
  if ([state isEqualToString:@"on"])   return [NSColor systemGreenColor];
  if ([state isEqualToString:@"warn"]) return [NSColor systemOrangeColor];
  if ([state isEqualToString:@"bad"])  return [NSColor systemRedColor];
  return [NSColor tertiaryLabelColor];
}

// NSView places subviews from the bottom left, so a stack inside a scroll view
// is laid out from the bottom. A flipped view places the origin at the top
// left.
@interface FlippedView : NSView
@end
@implementation FlippedView
- (BOOL)isFlipped { return YES; }
@end

@interface DotView : NSView
@property(strong) NSColor *color;
@end

@implementation DotView
- (void)drawRect:(NSRect)r {
  CGFloat d = 9;
  NSRect box = NSMakeRect(NSMidX(self.bounds) - d/2, NSMidY(self.bounds) - d/2, d, d);
  [self.color setFill];
  [[NSBezierPath bezierPathWithOvalInRect:box] fill];
  [[[NSColor labelColor] colorWithAlphaComponent:0.12] setStroke];
  NSBezierPath *ring = [NSBezierPath bezierPathWithOvalInRect:NSInsetRect(box, 0.5, 0.5)];
  ring.lineWidth = 1;
  [ring stroke];
}
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
  self.layer.borderColor = [[NSColor separatorColor] colorWithAlphaComponent:0.6].CGColor;
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
@property(strong) NSMutableArray<NSString *> *actionIds;
- (void)fire:(NSInteger)index;
@end

@implementation ClickableRow
- (BOOL)isFlipped { return YES; }
- (void)mouseDown:(NSEvent *)e { [self.owner fire:self.action]; }
- (void)updateLayer {
  self.layer.backgroundColor = self.selected
      ? [NSColor controlAccentColor].CGColor : [NSColor clearColor].CGColor;
}
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
  self.appearance = [NSSegmentedControl segmentedControlWithLabels:@[@"Auto", @"Dark", @"Light"]
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
  [sideBg addSubview:self.sidebar];
  [NSLayoutConstraint activateConstraints:@[
    [sideBg.widthAnchor constraintEqualToConstant:kSidebarWidth],
    [self.sidebar.topAnchor constraintEqualToAnchor:sideBg.topAnchor],
    [self.sidebar.leadingAnchor constraintEqualToAnchor:sideBg.leadingAnchor],
    [self.sidebar.trailingAnchor constraintEqualToAnchor:sideBg.trailingAnchor],
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
  self.headerBar.spacing = 10;
  self.headerBar.edgeInsets = NSEdgeInsetsMake(16, 24, 14, 20);
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

  NSView *root = [NSView new];
  [root addSubview:sideBg];
  [root addSubview:detail];
  sideBg.translatesAutoresizingMaskIntoConstraints = NO;
  self.window.contentView = root;

  [NSLayoutConstraint activateConstraints:@[
    [sideBg.topAnchor constraintEqualToAnchor:root.topAnchor],
    [sideBg.bottomAnchor constraintEqualToAnchor:root.bottomAnchor],
    [sideBg.leadingAnchor constraintEqualToAnchor:root.leadingAnchor],

    [detail.leadingAnchor constraintEqualToAnchor:sideBg.trailingAnchor],
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

- (NSButton *)buttonFor:(NSDictionary *)spec {
  NSButton *b = [NSButton buttonWithTitle:spec[@"title"] ?: @"" target:self action:@selector(clicked:)];
  b.bezelStyle = NSBezelStyleRounded;
  b.controlSize = NSControlSizeSmall;
  b.font = [NSFont systemFontOfSize:12];
  b.enabled = ![spec[@"disabled"] boolValue];
  // Only the primary action uses the prominent style.
  if ([spec[@"primary"] boolValue] && b.enabled) {
    b.bezelColor = [NSColor controlAccentColor];
    b.contentTintColor = [NSColor whiteColor];
  }
  b.tag = [self claim:spec[@"id"]];
  return b;
}

- (NSView *)hairline {
  NSView *v = [NSView new];
  v.wantsLayer = YES;
  v.layer.backgroundColor = [[NSColor separatorColor] colorWithAlphaComponent:0.5].CGColor;
  [v.heightAnchor constraintEqualToConstant:1].active = YES;
  return v;
}

- (NSTextField *)chip:(NSString *)text {
  NSTextField *t = [NSTextField labelWithString:text];
  t.font = [NSFont systemFontOfSize:11];
  t.textColor = [NSColor secondaryLabelColor];
  return t;
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
  NSStackView *titleWrap = [NSStackView new];
  titleWrap.edgeInsets = NSEdgeInsetsMake(12, 16, 4, 16);
  [titleWrap addArrangedSubview:title];
  [box addArrangedSubview:titleWrap];

  for (NSDictionary *item in group[@"items"]) {
    BOOL selected = [item[@"selected"] boolValue];
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
    line.edgeInsets = NSEdgeInsetsMake(6, 8, 6, 8);
    line.translatesAutoresizingMaskIntoConstraints = NO;

    NSString *dot = item[@"dot"];
    if (dot.length) {
      DotView *d = [[DotView alloc] initWithFrame:NSMakeRect(0, 0, 10, 10)];
      d.color = selected ? [NSColor whiteColor] : dotColor(dot);
      [d.widthAnchor constraintEqualToConstant:10].active = YES;
      [d.heightAnchor constraintEqualToConstant:10].active = YES;
      [line addArrangedSubview:d];
    }
    NSTextField *label = [NSTextField labelWithString:item[@"label"] ?: @""];
    label.font = [NSFont systemFontOfSize:13];
    label.lineBreakMode = NSLineBreakByTruncatingTail;
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

- (NSView *)kvRow:(NSDictionary *)row {
  NSStackView *line = [NSStackView new];
  line.orientation = NSUserInterfaceLayoutOrientationHorizontal;
  line.alignment = NSLayoutAttributeCenterY;
  line.spacing = 10;
  line.edgeInsets = NSEdgeInsetsMake(9, 14, 9, 12);

  NSTextField *key = [NSTextField labelWithString:row[@"text"] ?: @""];
  key.font = [NSFont systemFontOfSize:12];
  key.textColor = [NSColor secondaryLabelColor];
  [key.widthAnchor constraintEqualToConstant:116].active = YES;
  [line addArrangedSubview:key];

  NSTextField *val = [NSTextField labelWithString:row[@"detail"] ?: @""];
  val.font = [NSFont systemFontOfSize:12];
  val.lineBreakMode = NSLineBreakByTruncatingHead;
  [val setContentCompressionResistancePriority:NSLayoutPriorityDefaultLow
                                forOrientation:NSLayoutConstraintOrientationHorizontal];
  [line addArrangedSubview:val];

  NSView *spacer = [NSView new];
  [spacer setContentHuggingPriority:1 forOrientation:NSLayoutConstraintOrientationHorizontal];
  [line addArrangedSubview:spacer];
  for (NSDictionary *b in row[@"buttons"]) [line addArrangedSubview:[self buttonFor:b]];
  return line;
}

- (NSView *)rowFor:(NSDictionary *)row {
  if ([row[@"kind"] isEqualToString:@"kv"]) return [self kvRow:row];

  NSStackView *line = [NSStackView new];
  line.orientation = NSUserInterfaceLayoutOrientationHorizontal;
  line.alignment = NSLayoutAttributeCenterY;
  line.spacing = 10;
  line.edgeInsets = NSEdgeInsetsMake(9, 14, 9, 12);

  DotView *dot = [[DotView alloc] initWithFrame:NSMakeRect(0, 0, 10, 10)];
  dot.color = dotColor(row[@"dot"] ?: @"");
  [dot.widthAnchor constraintEqualToConstant:10].active = YES;
  [dot.heightAnchor constraintEqualToConstant:10].active = YES;
  [line addArrangedSubview:dot];

  NSTextField *name = [NSTextField labelWithString:row[@"text"] ?: @""];
  name.font = [NSFont systemFontOfSize:13];
  name.lineBreakMode = NSLineBreakByTruncatingTail;
  [name.widthAnchor constraintEqualToConstant:kNameWidth].active = YES;
  [line addArrangedSubview:name];

  NSString *chip = row[@"chip"];
  if (chip.length) [line addArrangedSubview:[self chip:chip]];

  // A row of dots reports the state of each service in a project.
  NSArray *dots = row[@"dots"];
  if (dots.count) {
    NSStackView *run = [NSStackView new];
    run.orientation = NSUserInterfaceLayoutOrientationHorizontal;
    run.spacing = 4;
    for (NSString *d in dots) {
      DotView *v = [[DotView alloc] initWithFrame:NSMakeRect(0, 0, 10, 10)];
      v.color = dotColor(d);
      [v.widthAnchor constraintEqualToConstant:10].active = YES;
      [v.heightAnchor constraintEqualToConstant:10].active = YES;
      [run addArrangedSubview:v];
    }
    [line addArrangedSubview:run];
  }

  // The address column holds a link when the row has a URL, and plain text
  // otherwise. Both use the same width.
  NSView *reach = [NSView new];
  NSString *link = row[@"link"];
  NSString *linkText = row[@"linkText"] ?: link;
  if (linkText.length) {
    NSView *inner;
    if (link.length) {
      NSButton *l = [NSButton buttonWithTitle:linkText target:self action:@selector(clicked:)];
      l.bezelStyle = NSBezelStyleInline;
      l.bordered = NO;
      l.contentTintColor = [NSColor linkColor];
      l.font = [NSFont systemFontOfSize:12];
      [l.cell setHighlightsBy:NSContentsCellMask];
      l.tag = [self claim:link];
      inner = l;
    } else {
      NSTextField *t = [NSTextField labelWithString:linkText];
      t.font = [NSFont systemFontOfSize:12];
      t.textColor = [NSColor secondaryLabelColor];
      t.lineBreakMode = NSLineBreakByTruncatingHead;
      inner = t;
    }
    inner.translatesAutoresizingMaskIntoConstraints = NO;
    [reach addSubview:inner];
    [NSLayoutConstraint activateConstraints:@[
      [inner.leadingAnchor constraintEqualToAnchor:reach.leadingAnchor],
      [inner.centerYAnchor constraintEqualToAnchor:reach.centerYAnchor],
      [inner.trailingAnchor constraintLessThanOrEqualToAnchor:reach.trailingAnchor],
    ]];
  }
  [reach.widthAnchor constraintEqualToConstant:kReachWidth].active = YES;
  [line addArrangedSubview:reach];

  NSView *spacer = [NSView new];
  [spacer setContentHuggingPriority:1 forOrientation:NSLayoutConstraintOrientationHorizontal];
  [line addArrangedSubview:spacer];

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
    if (note.length && [section[@"rows"] count] > 0) {
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
  if (!rows.count && note.length) {
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
  for (NSUInteger i = 0; i < rows.count; i++) {
    [inner addArrangedSubview:[self rowFor:rows[i]]];
    if (i + 1 < rows.count) [inner addArrangedSubview:[self hairline]];
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
  line.spacing = 10;

  DotView *dot = [[DotView alloc] initWithFrame:NSMakeRect(0, 0, 12, 12)];
  dot.color = dotColor(v[@"dot"] ?: @"on");
  [dot.widthAnchor constraintEqualToConstant:12].active = YES;
  [dot.heightAnchor constraintEqualToConstant:12].active = YES;
  [line addArrangedSubview:dot];

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

- (NSView *)bannerFor:(NSDictionary *)b {
  Card *card = [[Card alloc] initWithFrame:NSZeroRect];
  NSStackView *line = [NSStackView new];
  line.orientation = NSUserInterfaceLayoutOrientationHorizontal;
  line.alignment = NSLayoutAttributeCenterY;
  line.spacing = 14;
  line.edgeInsets = NSEdgeInsetsMake(13, 16, 13, 14);
  line.translatesAutoresizingMaskIntoConstraints = NO;

  NSStackView *text = [NSStackView new];
  text.orientation = NSUserInterfaceLayoutOrientationVertical;
  text.alignment = NSLayoutAttributeLeading;
  text.spacing = 3;
  NSTextField *title = [NSTextField labelWithString:b[@"title"] ?: @""];
  title.font = [NSFont systemFontOfSize:13 weight:NSFontWeightSemibold];
  [text addArrangedSubview:title];
  NSTextField *body = [NSTextField wrappingLabelWithString:b[@"text"] ?: @""];
  body.font = [NSFont systemFontOfSize:12];
  body.textColor = [NSColor secondaryLabelColor];
  [text addArrangedSubview:body];
  [line addArrangedSubview:text];

  NSView *spacer = [NSView new];
  [spacer setContentHuggingPriority:1 forOrientation:NSLayoutConstraintOrientationHorizontal];
  [line addArrangedSubview:spacer];
  if (b[@"button"]) [line addArrangedSubview:[self buttonFor:b[@"button"]]];

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

- (void)showTitle:(NSString *)title text:(NSString *)body {
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
  [self.text scrollRangeToVisible:NSMakeRange(self.text.string.length, 0)];
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
    [alert addButtonWithTitle:@"OK"];
    [alert addButtonWithTitle:@"Cancel"];

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
    [alert addButtonWithTitle:@"Cancel"];
    if (destructive) okButton.hasDestructiveAction = YES;
    if ([alert runModal] == NSAlertFirstButtonReturn) {
      goUIAction((char *)[aid UTF8String]);
    }
  });
}

void ui_logs(const char *title, const char *text) {
  NSString *t = [NSString stringWithUTF8String:title ?: ""];
  NSString *b = [NSString stringWithUTF8String:text ?: ""];
  dispatch_async(dispatch_get_main_queue(), ^{ [[LogWindow shared] showTitle:t text:b]; });
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
