#import "XDLFetchViewController.h"
#import "XDLCoreClient.h"
#import "XDLCardView.h"
#import "XDLComponents.h"
#import "XDLSettings.h"
#import "XDLTheme.h"

static const NSInteger kBatchSize = 100;

@interface XDLFetchViewController () <NSTableViewDataSource, NSTableViewDelegate, NSTextFieldDelegate>

@property (nonatomic) XDLCoreClient *client;

// Fetch form
@property (nonatomic) NSTextField *usernameField;
@property (nonatomic) NSPopUpButton *timelineTypePopup;
@property (nonatomic) XDLPillButton *photosChip;
@property (nonatomic) XDLPillButton *videosChip;
@property (nonatomic) XDLPillButton *gifsChip;
@property (nonatomic) XDLPillButton *textChip;
@property (nonatomic) XDLPillButton *retweetsChip;
@property (nonatomic) XDLPillButton *fetchButton;
@property (nonatomic) XDLPillButton *stopFetchButton;
@property (nonatomic) NSTextField *fetchStatusLabel;
@property (nonatomic) NSProgressIndicator *fetchSpinner;

// Results
@property (nonatomic) NSView *resultsCard;
@property (nonatomic) NSImageView *avatarView;
@property (nonatomic) NSTextField *accountLabel;
@property (nonatomic) NSTextField *statsLabel;
@property (nonatomic, copy, nullable) NSString *avatarURL;
@property (nonatomic) NSTableView *tableView;
@property (nonatomic) XDLPillButton *downloadButton;
@property (nonatomic) XDLPillButton *stopDownloadButton;
@property (nonatomic) XDLPillButton *openFolderButton;
@property (nonatomic) NSProgressIndicator *downloadBar;
@property (nonatomic) NSTextField *downloadStatusLabel;

// State
@property (nonatomic) NSMutableArray<NSDictionary *> *items;
@property (nonatomic) NSMutableDictionary<NSNumber *, NSString *> *itemStatuses; // index -> status
@property (nonatomic) NSMutableDictionary<NSNumber *, NSString *> *itemPaths;    // index -> downloaded file
@property (nonatomic, copy, nullable) NSString *planSummary; // "X downloaded · Y to download"
@property (nonatomic, nullable) NSDictionary *accountInfo;
@property (nonatomic, copy) NSString *activeHandle;
@property (nonatomic, copy) NSString *activeTimelineType;
@property (nonatomic) BOOL fetching;
@property (nonatomic) BOOL stopRequested;
@property (nonatomic) BOOL downloading;
@property (nonatomic, copy, nullable) NSString *cursor;
@property (nonatomic) NSInteger page;

// Autocomplete from previously saved accounts
@property (nonatomic) NSArray<NSString *> *knownUsernames;
@property (nonatomic) BOOL completingUsername;

@end

@implementation XDLFetchViewController

- (instancetype)initWithClient:(XDLCoreClient *)client {
    self = [super initWithTitle:@"Download"];
    if (self) {
        _client = client;
        _items = [NSMutableArray array];
        _itemStatuses = [NSMutableDictionary dictionary];
        _itemPaths = [NSMutableDictionary dictionary];
    }
    return self;
}

- (void)viewDidLoad {
    [super viewDidLoad];
    [self buildFetchCard];
    [self buildResultsCard];

    [NSNotificationCenter.defaultCenter addObserver:self
                                           selector:@selector(coreEvent:)
                                               name:XDLCoreEventNotification
                                             object:nil];
    [NSNotificationCenter.defaultCenter addObserver:self
                                           selector:@selector(coreStateChanged:)
                                               name:XDLCoreStateDidChangeNotification
                                             object:nil];
    [self updateControlAvailability];
}

- (void)viewDidAppear {
    [super viewDidAppear];
    [self refreshKnownUsernames];
}

- (void)dealloc {
    [NSNotificationCenter.defaultCenter removeObserver:self];
}

#pragma mark - UI construction

// A toggle chip: pill with a dot that lights up when selected; writes the
// choice straight back into settings.
- (XDLPillButton *)chipWithTitle:(NSString *)title on:(BOOL)on changed:(void (^)(BOOL))changed {
    XDLPillButton *chip = [XDLPillButton pillWithTitle:title onClick:nil];
    chip.selected = on;
    chip.dotColor = on ? XDLTheme.accentBlue : nil;
    __weak XDLPillButton *weakChip = chip;
    chip.onClick = ^{
        BOOL now = !weakChip.selected;
        weakChip.selected = now;
        weakChip.dotColor = now ? XDLTheme.accentBlue : nil;
        changed(now);
    };
    return chip;
}

- (NSStackView *)fieldColumnWithCaption:(NSString *)caption view:(NSView *)view {
    NSStackView *column = [NSStackView stackViewWithViews:@[
        [XDLCaptionLabel captionWithText:caption], view
    ]];
    column.orientation = NSUserInterfaceLayoutOrientationVertical;
    column.alignment = NSLayoutAttributeLeading;
    column.spacing = 5;
    return column;
}

- (void)buildFetchCard {
    NSStackView *stack = [self addCardWithCaption:@"New Fetch"
                                      belowAnchor:self.contentTopAnchor
                                              gap:6
                                          outCard:NULL];
    stack.spacing = 14;

    // Username + timeline type, each under its own caption.
    self.usernameField = [XDLUI fieldWithPlaceholder:@"@username or profile URL"];
    self.usernameField.delegate = self;
    self.usernameField.controlSize = NSControlSizeLarge;
    self.usernameField.font = [NSFont systemFontOfSize:13.5];
    [self.usernameField setContentHuggingPriority:NSLayoutPriorityDefaultLow
                                   forOrientation:NSLayoutConstraintOrientationHorizontal];
    self.timelineTypePopup = [XDLUI popupWithItems:@[
        @"Media", @"Timeline", @"Tweets", @"With Replies", @"Likes", @"Bookmarks"
    ]];
    self.timelineTypePopup.controlSize = NSControlSizeLarge;
    [self.timelineTypePopup setContentHuggingPriority:NSLayoutPriorityDefaultHigh
                                       forOrientation:NSLayoutConstraintOrientationHorizontal];

    NSStackView *fieldColumn = [self fieldColumnWithCaption:@"Username" view:self.usernameField];
    NSStackView *typeColumn = [self fieldColumnWithCaption:@"Timeline" view:self.timelineTypePopup];
    [self.usernameField.widthAnchor constraintEqualToAnchor:fieldColumn.widthAnchor].active = YES;
    NSStackView *row1 = [NSStackView stackViewWithViews:@[ fieldColumn, typeColumn ]];
    row1.spacing = 14;
    row1.alignment = NSLayoutAttributeBottom;
    row1.distribution = NSStackViewDistributionFill;
    [fieldColumn setContentHuggingPriority:NSLayoutPriorityDefaultLow
                            forOrientation:NSLayoutConstraintOrientationHorizontal];
    [stack addArrangedSubview:row1];
    [row1.widthAnchor constraintEqualToAnchor:stack.widthAnchor].active = YES;

    __weak typeof(self) weakSelf = self;

    // Media type toggle chips.
    XDLSettings *settings = XDLSettings.shared;
    self.photosChip = [self chipWithTitle:@"Photos" on:settings.includePhotos changed:^(BOOL on) {
        XDLSettings.shared.includePhotos = on;
    }];
    self.videosChip = [self chipWithTitle:@"Videos" on:settings.includeVideos changed:^(BOOL on) {
        XDLSettings.shared.includeVideos = on;
    }];
    self.gifsChip = [self chipWithTitle:@"GIFs" on:settings.includeGifs changed:^(BOOL on) {
        XDLSettings.shared.includeGifs = on;
    }];
    self.textChip = [self chipWithTitle:@"Text" on:settings.includeText changed:^(BOOL on) {
        XDLSettings.shared.includeText = on;
    }];
    self.retweetsChip = [self chipWithTitle:@"Retweets" on:settings.includeRetweets changed:^(BOOL on) {
        XDLSettings.shared.includeRetweets = on;
    }];
    NSStackView *chipsRow = [NSStackView stackViewWithViews:@[
        self.photosChip, self.videosChip, self.gifsChip, self.textChip, self.retweetsChip
    ]];
    chipsRow.spacing = 8;
    NSStackView *chipsColumn = [self fieldColumnWithCaption:@"Media Types" view:chipsRow];
    [stack addArrangedSubview:chipsColumn];

    NSView *separator = [XDLUI separator];
    [stack addArrangedSubview:separator];
    [separator.widthAnchor constraintEqualToAnchor:stack.widthAnchor].active = YES;

    // Action row: buttons left, live status right.
    self.fetchButton = [XDLPillButton pillWithTitle:@"Fetch" onClick:^{
        [weakSelf startFetch];
    }];
    self.fetchButton.dotColor = XDLTheme.accentBlue;
    self.stopFetchButton = [XDLPillButton pillWithTitle:@"Stop" onClick:^{
        weakSelf.stopRequested = YES;
        [weakSelf.client post:@"/extract/cancel" body:nil completion:nil];
    }];
    self.stopFetchButton.enabled = NO;
    self.fetchSpinner = [[NSProgressIndicator alloc] initWithFrame:NSZeroRect];
    self.fetchSpinner.style = NSProgressIndicatorStyleSpinning;
    self.fetchSpinner.controlSize = NSControlSizeSmall;
    self.fetchSpinner.displayedWhenStopped = NO;
    self.fetchSpinner.translatesAutoresizingMaskIntoConstraints = NO;
    self.fetchStatusLabel = [XDLUI labelWithText:@"" size:11.5 weight:NSFontWeightRegular color:XDLTheme.secondaryLabel];
    self.fetchStatusLabel.alignment = NSTextAlignmentRight;
    // The status label is the designated flex view; the pills hug their content.
    [self.fetchStatusLabel setContentHuggingPriority:100
                                      forOrientation:NSLayoutConstraintOrientationHorizontal];
    NSStackView *row4 = [NSStackView stackViewWithViews:@[
        self.fetchButton, self.stopFetchButton, self.fetchSpinner, self.fetchStatusLabel
    ]];
    row4.spacing = 10;
    row4.distribution = NSStackViewDistributionFill;
    [stack addArrangedSubview:row4];
    [row4.widthAnchor constraintEqualToAnchor:stack.widthAnchor].active = YES;
}

- (void)buildResultsCard {
    NSView *card = nil;
    NSStackView *stack = [self addCardWithCaption:@"Results"
                                      belowAnchor:[self lastCardBottomAnchor]
                                              gap:18
                                          outCard:&card];
    self.resultsCard = card;

    // Profile header: avatar + name/handle + stats
    self.avatarView = [[NSImageView alloc] initWithFrame:NSZeroRect];
    self.avatarView.translatesAutoresizingMaskIntoConstraints = NO;
    self.avatarView.wantsLayer = YES;
    self.avatarView.layer.cornerRadius = 22;
    self.avatarView.layer.masksToBounds = YES;
    self.avatarView.imageScaling = NSImageScaleProportionallyUpOrDown;
    self.avatarView.hidden = YES;
    [NSLayoutConstraint activateConstraints:@[
        [self.avatarView.widthAnchor constraintEqualToConstant:44],
        [self.avatarView.heightAnchor constraintEqualToConstant:44],
    ]];

    self.accountLabel = [XDLUI labelWithText:@"No data yet — fetch an account above."
                                        size:13.5
                                      weight:NSFontWeightSemibold
                                       color:XDLTheme.primaryText];
    self.statsLabel = [XDLUI labelWithText:@"" size:11.5 weight:NSFontWeightRegular color:XDLTheme.secondaryLabel];

    NSStackView *nameStack = [NSStackView stackViewWithViews:@[ self.accountLabel, self.statsLabel ]];
    nameStack.orientation = NSUserInterfaceLayoutOrientationVertical;
    nameStack.alignment = NSLayoutAttributeLeading;
    nameStack.spacing = 2;

    NSStackView *header = [NSStackView stackViewWithViews:@[ self.avatarView, nameStack ]];
    header.spacing = 10;
    [stack addArrangedSubview:header];

    // Table
    self.tableView = [[NSTableView alloc] initWithFrame:NSZeroRect];
    self.tableView.dataSource = self;
    self.tableView.delegate = self;
    self.tableView.rowHeight = 24;
    self.tableView.usesAlternatingRowBackgroundColors = NO;
    self.tableView.backgroundColor = NSColor.clearColor;
    self.tableView.headerView = [[NSTableHeaderView alloc] init];

    struct {
        const char *identifier;
        const char *title;
        CGFloat width;
    } columns[] = {
        { "type", "Type", 70 },
        { "date", "Date", 130 },
        { "tweet", "Tweet ID", 150 },
        { "file", "File", 220 },
        { "status", "Status", 90 },
    };
    for (size_t i = 0; i < sizeof(columns) / sizeof(columns[0]); i++) {
        NSTableColumn *column = [[NSTableColumn alloc]
            initWithIdentifier:@(columns[i].identifier)];
        column.title = @(columns[i].title);
        column.width = columns[i].width;
        [self.tableView addTableColumn:column];
    }

    NSMenu *menu = [[NSMenu alloc] init];
    NSMenuItem *finderItem = [menu addItemWithTitle:@"Show in Finder"
                                             action:@selector(showItemInFinder:)
                                      keyEquivalent:@""];
    finderItem.target = self;
    NSMenuItem *tweetItem = [menu addItemWithTitle:@"Open Tweet on X"
                                            action:@selector(openItemTweet:)
                                     keyEquivalent:@""];
    tweetItem.target = self;
    self.tableView.menu = menu;

    NSScrollView *tableScroll = [[NSScrollView alloc] initWithFrame:NSZeroRect];
    tableScroll.documentView = self.tableView;
    tableScroll.hasVerticalScroller = YES;
    tableScroll.drawsBackground = NO;
    tableScroll.borderType = NSNoBorder;
    tableScroll.translatesAutoresizingMaskIntoConstraints = NO;
    // Grows with the window; the surrounding card is pinned to the page bottom.
    [tableScroll setContentHuggingPriority:100
                            forOrientation:NSLayoutConstraintOrientationVertical];
    [stack addArrangedSubview:tableScroll];
    [NSLayoutConstraint activateConstraints:@[
        [tableScroll.heightAnchor constraintGreaterThanOrEqualToConstant:280],
        [tableScroll.widthAnchor constraintEqualToAnchor:stack.widthAnchor],
    ]];

    // Download controls
    __weak typeof(self) weakSelf = self;
    self.downloadButton = [XDLPillButton pillWithTitle:@"Download All" onClick:^{
        [weakSelf startDownload];
    }];
    self.stopDownloadButton = [XDLPillButton pillWithTitle:@"Stop" onClick:^{
        [weakSelf.client post:@"/download/stop" body:nil completion:nil];
    }];
    self.stopDownloadButton.enabled = NO;
    self.openFolderButton = [XDLPillButton pillWithTitle:@"Open Folder" onClick:^{
        [weakSelf openDownloadFolder];
    }];
    self.downloadBar = [[NSProgressIndicator alloc] initWithFrame:NSZeroRect];
    self.downloadBar.style = NSProgressIndicatorStyleBar;
    self.downloadBar.indeterminate = NO;
    self.downloadBar.minValue = 0;
    self.downloadBar.doubleValue = 0;
    self.downloadBar.translatesAutoresizingMaskIntoConstraints = NO;
    self.downloadStatusLabel = [XDLUI labelWithText:@"" size:11.5 weight:NSFontWeightRegular color:XDLTheme.secondaryLabel];

    NSStackView *controls = [NSStackView stackViewWithViews:@[
        self.downloadButton, self.stopDownloadButton, self.openFolderButton,
        self.downloadBar, self.downloadStatusLabel
    ]];
    controls.spacing = 10;
    controls.distribution = NSStackViewDistributionFill;
    // The progress bar absorbs the leftover width; buttons stay compact.
    [self.downloadBar setContentHuggingPriority:100
                                 forOrientation:NSLayoutConstraintOrientationHorizontal];
    [stack addArrangedSubview:controls];
    [NSLayoutConstraint activateConstraints:@[
        [controls.widthAnchor constraintEqualToAnchor:stack.widthAnchor],
        [self.downloadBar.widthAnchor constraintGreaterThanOrEqualToConstant:160],
    ]];

    // Fill the viewport so the bottom-pinned card stretches with the window,
    // keeping a real margin under the controls row.
    [NSLayoutConstraint activateConstraints:@[
        [self.contentView.heightAnchor constraintGreaterThanOrEqualToAnchor:self.viewportHeightAnchor],
        [card.bottomAnchor constraintEqualToAnchor:self.contentView.bottomAnchor constant:-XDLPageContentMargin],
    ]];

    [self updateResultControls];
}

// The fetch card is the first card; track its bottom via the last subview added.
- (NSLayoutYAxisAnchor *)lastCardBottomAnchor {
    NSView *last = self.contentView.subviews.lastObject;
    return last ? last.bottomAnchor : self.contentTopAnchor;
}

#pragma mark - Username autocomplete

- (void)refreshKnownUsernames {
    if (self.client.state != XDLCoreStateReady) {
        return;
    }
    __weak typeof(self) weakSelf = self;
    [self.client get:@"/accounts" completion:^(id json, NSError *error) {
        if (error || ![json isKindOfClass:NSArray.class]) {
            return;
        }
        NSMutableOrderedSet<NSString *> *usernames = [NSMutableOrderedSet orderedSet];
        for (NSDictionary *account in json) {
            NSString *username = [account isKindOfClass:NSDictionary.class] ? account[@"username"] : nil;
            if ([username isKindOfClass:NSString.class] && username.length > 0) {
                [usernames addObject:username];
            }
        }
        weakSelf.knownUsernames = usernames.array;
    }];
}

- (void)controlTextDidChange:(NSNotification *)notification {
    if (notification.object != self.usernameField || self.completingUsername) {
        return;
    }
    NSTextView *editor = (NSTextView *)self.usernameField.currentEditor;
    if (![editor isKindOfClass:NSTextView.class] || self.usernameField.stringValue.length == 0) {
        return;
    }
    self.completingUsername = YES;
    [editor complete:nil];
    self.completingUsername = NO;
}

- (NSArray<NSString *> *)control:(NSControl *)control
                        textView:(NSTextView *)textView
                     completions:(NSArray<NSString *> *)words
             forPartialWordRange:(NSRange)charRange
             indexOfSelectedItem:(NSInteger *)index {
    if (control != self.usernameField) {
        return words;
    }
    NSString *partial = [[textView.string substringWithRange:charRange] lowercaseString];
    if (partial.length == 0) {
        return @[];
    }
    NSMutableArray<NSString *> *matches = [NSMutableArray array];
    for (NSString *username in self.knownUsernames) {
        if ([username.lowercaseString hasPrefix:partial]) {
            [matches addObject:username];
        }
    }
    // Already fully typed — don't pop a one-item list over the field.
    if (matches.count == 1 && [matches[0] caseInsensitiveCompare:partial] == NSOrderedSame) {
        return @[];
    }
    *index = -1;
    return matches;
}

#pragma mark - Fetch

- (NSString *)selectedTimelineType {
    switch (self.timelineTypePopup.indexOfSelectedItem) {
        case 0: return @"media";
        case 1: return @"timeline";
        case 2: return @"tweets";
        case 3: return @"with_replies";
        case 4: return @"likes";
        case 5: return @"bookmarks";
    }
    return @"media";
}

+ (NSString *)cleanHandle:(NSString *)raw {
    NSString *handle = [raw stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet];
    for (NSString *prefix in @[ @"https://", @"http://" ]) {
        if ([handle hasPrefix:prefix]) {
            handle = [handle substringFromIndex:prefix.length];
        }
    }
    for (NSString *host in @[ @"x.com/", @"twitter.com/", @"www.x.com/", @"www.twitter.com/" ]) {
        if ([handle hasPrefix:host]) {
            handle = [handle substringFromIndex:host.length];
        }
    }
    NSRange slash = [handle rangeOfString:@"/"];
    if (slash.location != NSNotFound) {
        handle = [handle substringToIndex:slash.location];
    }
    if ([handle hasPrefix:@"@"]) {
        handle = [handle substringFromIndex:1];
    }
    return handle;
}

- (void)startFetch {
    if (self.fetching || self.downloading) {
        return;
    }
    NSString *timelineType = [self selectedTimelineType];
    NSString *handle = [XDLFetchViewController cleanHandle:self.usernameField.stringValue];

    if (handle.length == 0 && ![timelineType isEqualToString:@"bookmarks"]) {
        [XDLFetchViewController presentMessage:@"Please enter a username."];
        return;
    }

    // The token lives in the Keychain (Settings → X Account).
    __weak typeof(self) weakSelf = self;
    [self.client get:@"/authtoken?slot=public" completion:^(id json, NSError *error) {
        __strong typeof(weakSelf) self = weakSelf;
        if (!self || self.fetching || self.downloading) return;
        NSString *token = [json isKindOfClass:NSDictionary.class] ? json[@"token"] : nil;
        if (![token isKindOfClass:NSString.class] || token.length == 0) {
            [XDLFetchViewController presentMessage:
                @"No auth token saved. Add your X auth_token in Settings → X Account first."];
            return;
        }
        [self beginFetchWithHandle:handle token:token timelineType:timelineType];
    }];
}

- (void)beginFetchWithHandle:(NSString *)handle
                       token:(NSString *)token
                timelineType:(NSString *)timelineType {
    self.fetching = YES;
    self.stopRequested = NO;
    self.cursor = nil;
    self.page = 0;
    self.activeHandle = [timelineType isEqualToString:@"bookmarks"] ? @"bookmarks"
                       : [timelineType isEqualToString:@"likes"] ? handle
                       : handle;
    self.activeTimelineType = timelineType;
    self.accountInfo = nil;
    self.planSummary = nil;
    [self.items removeAllObjects];
    [self.itemStatuses removeAllObjects];
    [self.itemPaths removeAllObjects];
    [self.tableView reloadData];
    [self updateAccountLabel];
    [self resetDownloadProgress];
    [self.fetchSpinner startAnimation:nil];
    self.fetchStatusLabel.stringValue = @"Fetching…";
    [self updateControlAvailability];
    [self fetchNextPageWithHandle:handle token:token timelineType:timelineType];
}

- (void)fetchNextPageWithHandle:(NSString *)handle
                          token:(NSString *)token
                   timelineType:(NSString *)timelineType {
    XDLSettings *settings = XDLSettings.shared;
    self.page += 1;

    NSMutableDictionary *body = [@{
        @"username": handle ?: @"",
        @"auth_token": token,
        @"timeline_type": timelineType,
        @"batch_size": @(kBatchSize),
        @"page": @(self.page),
        @"media_type": timelineType,
        // Real JSON booleans, not 0/1 — the Go side rejects numbers here.
        @"retweets": self.retweetsChip.selected ? @YES : @NO,
        @"include_photos": self.photosChip.selected ? @YES : @NO,
        @"include_videos": self.videosChip.selected ? @YES : @NO,
        @"include_gifs": self.gifsChip.selected ? @YES : @NO,
        @"include_text": self.textChip.selected ? @YES : @NO,
        @"video_quality": settings.videoQuality,
        @"image_size": settings.imageSize,
    } mutableCopy];
    if (self.cursor.length > 0) {
        body[@"cursor"] = self.cursor;
    }

    __weak typeof(self) weakSelf = self;
    NSTimeInterval timeout = (NSTimeInterval)settings.fetchTimeout;
    [self.client post:@"/extract/timeline" body:body timeout:timeout completion:^(id json, NSError *error) {
        __strong typeof(weakSelf) self = weakSelf;
        if (!self) return;

        if (error) {
            BOOL timedOut = [error.domain isEqualToString:NSURLErrorDomain] && error.code == NSURLErrorTimedOut;
            if (timedOut) {
                // The request died client-side; make sure the extractor
                // process doesn't keep running server-side.
                [self.client post:@"/extract/cancel" body:nil completion:nil];
                [self finishFetchWithMessage:[NSString stringWithFormat:@"Timed out after %lds — partial results kept",
                                              (long)settings.fetchTimeout]];
            } else {
                [self finishFetchWithMessage:[NSString stringWithFormat:@"Failed: %@", error.localizedDescription]];
            }
            if (self.items.count == 0 && !timedOut) {
                [XDLFetchViewController presentError:error];
            }
            return;
        }
        if (![json isKindOfClass:NSDictionary.class]) {
            [self finishFetchWithMessage:@"Failed: malformed response"];
            return;
        }

        NSDictionary *response = json;
        if ([response[@"account_info"] isKindOfClass:NSDictionary.class]) {
            self.accountInfo = response[@"account_info"];
        }
        NSArray *timeline = response[@"timeline"];
        NSInteger before = self.items.count;
        if ([timeline isKindOfClass:NSArray.class]) {
            for (NSDictionary *entry in timeline) {
                if ([entry isKindOfClass:NSDictionary.class]) {
                    [self.items addObject:entry];
                }
            }
        }
        [self.tableView reloadData];
        [self updateAccountLabel];

        NSDictionary *metadata = [response[@"metadata"] isKindOfClass:NSDictionary.class] ? response[@"metadata"] : @{};
        NSString *nextCursor = metadata[@"cursor"] ?: response[@"cursor"];
        BOOL hasMore = [metadata[@"has_more"] boolValue];
        BOOL progressed = self.items.count > before;
        BOOL cursorLooped = [nextCursor isKindOfClass:NSString.class] && [nextCursor isEqualToString:self.cursor];

        self.fetchStatusLabel.stringValue = [NSString stringWithFormat:@"Page %ld — %lu items",
                                             (long)self.page, (unsigned long)self.items.count];

        if (hasMore && !self.stopRequested && progressed && !cursorLooped &&
            [nextCursor isKindOfClass:NSString.class] && nextCursor.length > 0) {
            self.cursor = nextCursor;
            [self fetchNextPageWithHandle:handle token:token timelineType:timelineType];
            return;
        }

        self.cursor = [nextCursor isKindOfClass:NSString.class] ? nextCursor : nil;
        BOOL completed = !hasMore;
        [self saveFetchedAccountCompleted:completed response:response];
        [self finishFetchWithMessage:[NSString stringWithFormat:@"%@ %lu items (%ld pages)",
                                      self.stopRequested ? @"Stopped —" : @"Done —",
                                      (unsigned long)self.items.count, (long)self.page]];
    }];
}

- (void)finishFetchWithMessage:(NSString *)message {
    self.fetching = NO;
    [self.fetchSpinner stopAnimation:nil];
    self.fetchStatusLabel.stringValue = message;
    [self updateControlAvailability];
    [self updateResultControls];
    [self checkLocalFiles];
}

- (void)saveFetchedAccountCompleted:(BOOL)completed response:(NSDictionary *)lastResponse {
    if (self.items.count == 0) {
        return;
    }
    NSDictionary *account = self.accountInfo ?: @{};
    // Store the merged timeline so Library reload shows everything.
    NSDictionary *merged = @{
        @"account_info": account,
        @"total_urls": @(self.items.count),
        @"timeline": self.items,
        @"metadata": lastResponse[@"metadata"] ?: @{},
        @"cursor": self.cursor ?: @"",
        @"completed": completed ? @YES : @NO,
    };
    NSData *data = [NSJSONSerialization dataWithJSONObject:merged options:0 error:nil];
    NSString *responseJSON = data ? [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding] : @"{}";

    [self.client post:@"/accounts/save" body:@{
        @"username": self.activeHandle ?: @"",
        @"name": account[@"nick"] ?: @"",
        @"profile_image": account[@"profile_image"] ?: @"",
        @"total_media": @(self.items.count),
        @"response_json": responseJSON,
        @"media_type": self.activeTimelineType ?: @"media",
        @"cursor": self.cursor ?: @"",
        @"completed": completed ? @YES : @NO,
    } completion:nil];
}

#pragma mark - Saved accounts

- (void)refetchAccount:(NSString *)username mediaType:(NSString *)mediaType {
    (void)self.view;
    self.usernameField.stringValue = username ?: @"";
    NSArray *types = @[ @"media", @"timeline", @"tweets", @"with_replies", @"likes", @"bookmarks" ];
    NSUInteger typeIndex = [types indexOfObject:mediaType ?: @""];
    [self.timelineTypePopup selectItemAtIndex:typeIndex == NSNotFound ? 0 : typeIndex];
    [self startFetch];
}

- (void)loadSavedAccount:(NSString *)username mediaType:(NSString *)mediaType {
    (void)self.view; // force -loadView so outlets exist
    NSString *path = [NSString stringWithFormat:@"/accounts/saved?username=%@&media_type=%@",
        [username stringByAddingPercentEncodingWithAllowedCharacters:NSCharacterSet.URLQueryAllowedCharacterSet],
        [mediaType stringByAddingPercentEncodingWithAllowedCharacters:NSCharacterSet.URLQueryAllowedCharacterSet]];
    __weak typeof(self) weakSelf = self;
    [self.client get:path completion:^(id json, NSError *error) {
        __strong typeof(weakSelf) self = weakSelf;
        if (!self) return;
        if (error) {
            [XDLFetchViewController presentError:error];
            return;
        }
        NSDictionary *response = [json isKindOfClass:NSDictionary.class] ? json[@"response_json"] : nil;
        if (![response isKindOfClass:NSDictionary.class]) {
            [XDLFetchViewController presentMessage:@"Saved account data is malformed."];
            return;
        }
        self.activeHandle = username;
        self.activeTimelineType = mediaType;
        self.accountInfo = [response[@"account_info"] isKindOfClass:NSDictionary.class] ? response[@"account_info"] : nil;
        [self.items removeAllObjects];
        for (NSDictionary *entry in response[@"timeline"]) {
            if ([entry isKindOfClass:NSDictionary.class]) {
                [self.items addObject:entry];
            }
        }
        [self.itemStatuses removeAllObjects];
        [self.itemPaths removeAllObjects];
        [self.tableView reloadData];
        [self updateAccountLabel];
        [self updateResultControls];
        [self resetDownloadProgress];
        self.fetchStatusLabel.stringValue = [NSString stringWithFormat:@"Loaded @%@ from library", username];
        [self checkLocalFiles];
    }];
}

#pragma mark - Download

// Shared by the real download and the /download/plan dry run — both must see
// the identical request or the "already downloaded" check would drift.
- (NSDictionary *)downloadRequestBody {
    XDLSettings *settings = XDLSettings.shared;
    NSString *accountName = self.accountInfo[@"nick"] ?: @"";

    NSMutableArray *requestItems = [NSMutableArray arrayWithCapacity:self.items.count];
    for (NSDictionary *entry in self.items) {
        NSMutableDictionary *item = [@{
            @"url": entry[@"url"] ?: @"",
            @"date": entry[@"date"] ?: @"",
            @"tweet_id": entry[@"tweet_id"] ?: @"0",
            @"type": entry[@"type"] ?: @"photo",
        } mutableCopy];
        if ([entry[@"content"] isKindOfClass:NSString.class]) {
            item[@"content"] = entry[@"content"];
        }
        if ([entry[@"original_filename"] isKindOfClass:NSString.class]) {
            item[@"original_filename"] = entry[@"original_filename"];
        }
        if ([entry[@"author_username"] isKindOfClass:NSString.class]) {
            item[@"author_username"] = entry[@"author_username"];
        }
        item[@"account_name"] = accountName;
        [requestItems addObject:item];
    }

    return @{
        @"items": requestItems,
        @"output_dir": settings.downloadPath,
        @"username": self.activeHandle ?: @"",
        @"concurrent_downloads": @(settings.concurrentDownloads),
        @"skip_existing": settings.skipExistingFiles ? @YES : @NO,
        @"delete_incomplete_files": settings.deleteIncompleteFiles ? @YES : @NO,
        @"retry_attempts": @(settings.retryAttempts),
        @"proxy": settings.proxy,
        @"filename_template": settings.filenameTemplate,
        @"folder_template": settings.folderTemplate,
        @"auto_convert_gifs": settings.autoConvertGifs ? @YES : @NO,
        @"gif_quality": settings.gifQuality,
        @"gif_resolution": settings.gifResolution,
    };
}

// Dry-run the download to mark items whose files already exist on disk —
// a refetch may return fewer items than are stored locally (deleted posts),
// so the comparison is per fetched item, not by count.
- (void)checkLocalFiles {
    if (self.items.count == 0 || self.downloading || self.client.state != XDLCoreStateReady) {
        return;
    }
    __weak typeof(self) weakSelf = self;
    [self.client post:@"/download/plan" body:[self downloadRequestBody] completion:^(id json, NSError *error) {
        __strong typeof(weakSelf) self = weakSelf;
        if (!self || self.downloading || error || ![json isKindOfClass:NSDictionary.class]) {
            return;
        }
        NSInteger existing = 0;
        for (NSDictionary *plan in json[@"items"]) {
            if (![plan isKindOfClass:NSDictionary.class]) continue;
            NSInteger index = [plan[@"index"] integerValue];
            if (index < 0 || index >= (NSInteger)self.items.count) continue;
            if ([plan[@"exists"] boolValue]) {
                existing += 1;
                self.itemStatuses[@(index)] = @"downloaded";
                NSString *path = plan[@"path"];
                if ([path isKindOfClass:NSString.class] && path.length > 0) {
                    self.itemPaths[@(index)] = path;
                }
            }
        }
        NSInteger total = (NSInteger)self.items.count;
        self.planSummary = [NSString stringWithFormat:@"%ld downloaded · %ld to download",
                            (long)existing, (long)(total - existing)];
        [self.tableView reloadData];
        [self updateAccountLabel];
    }];
}

- (void)startDownload {
    if (self.downloading || self.items.count == 0) {
        return;
    }
    NSDictionary *body = [self downloadRequestBody];

    self.downloading = YES;
    [self.itemStatuses removeAllObjects];
    [self.itemPaths removeAllObjects];
    [self.tableView reloadData];
    self.downloadBar.maxValue = self.items.count;
    self.downloadBar.doubleValue = 0;
    self.downloadStatusLabel.stringValue = [NSString stringWithFormat:@"0/%lu", (unsigned long)self.items.count];
    [self updateControlAvailability];
    [self updateResultControls];

    __weak typeof(self) weakSelf = self;
    [self.client post:@"/download" body:body completion:^(id json, NSError *error) {
        __strong typeof(weakSelf) self = weakSelf;
        if (!self) return;
        if (error) {
            self.downloading = NO;
            self.downloadStatusLabel.stringValue = error.localizedDescription;
            [self updateControlAvailability];
            [self updateResultControls];
            [XDLFetchViewController presentError:error];
        }
    }];
}

#pragma mark - Core events

- (void)coreEvent:(NSNotification *)notification {
    NSString *type = notification.userInfo[@"type"];
    id data = notification.userInfo[@"data"];

    if ([type isEqualToString:@"download-progress"] && [data isKindOfClass:NSDictionary.class]) {
        NSInteger current = [data[@"current"] integerValue];
        NSInteger total = [data[@"total"] integerValue];
        self.downloadBar.maxValue = MAX(1, total);
        self.downloadBar.doubleValue = current;
        self.downloadStatusLabel.stringValue = [NSString stringWithFormat:@"%ld/%ld", (long)current, (long)total];
        return;
    }
    if ([type isEqualToString:@"download-item-status"] && [data isKindOfClass:NSDictionary.class]) {
        NSInteger index = [data[@"index"] integerValue];
        NSString *status = data[@"status"];
        if ([status isKindOfClass:NSString.class] && index >= 0 && index < (NSInteger)self.items.count) {
            self.itemStatuses[@(index)] = status;
            NSString *path = data[@"path"];
            if ([path isKindOfClass:NSString.class] && path.length > 0 &&
                ([status isEqualToString:@"success"] || [status isEqualToString:@"skipped"])) {
                self.itemPaths[@(index)] = path;
            }
            [self.tableView reloadDataForRowIndexes:[NSIndexSet indexSetWithIndex:index]
                                      columnIndexes:[NSIndexSet indexSetWithIndex:4]];
        }
        return;
    }
    if ([type isEqualToString:@"download-finished"] && [data isKindOfClass:NSDictionary.class]) {
        self.downloading = NO;
        NSString *message = [data[@"message"] isKindOfClass:NSString.class] ? data[@"message"] : @"Finished";
        self.downloadStatusLabel.stringValue = message;
        [self updateControlAvailability];
        [self updateResultControls];
        [self checkLocalFiles];
        return;
    }
}

- (void)coreStateChanged:(NSNotification *)notification {
    [self updateControlAvailability];
}

#pragma mark - Helpers

- (void)updateAccountLabel {
    if (!self.accountInfo && self.items.count == 0) {
        self.accountLabel.stringValue = @"No data yet — fetch an account above.";
        self.statsLabel.stringValue = @"";
        self.avatarView.hidden = YES;
        return;
    }
    NSString *nick = self.accountInfo[@"nick"] ?: @"";
    NSString *name = self.activeHandle ?: @"";
    self.accountLabel.stringValue = [NSString stringWithFormat:@"%@ (@%@)",
                                     nick.length ? nick : name, name];

    NSMutableArray<NSString *> *parts = [NSMutableArray array];
    NSNumber *followers = self.accountInfo[@"followers_count"];
    NSNumber *statuses = self.accountInfo[@"statuses_count"];
    if ([followers isKindOfClass:NSNumber.class]) {
        [parts addObject:[NSString stringWithFormat:@"%@ followers", followers]];
    }
    if ([statuses isKindOfClass:NSNumber.class]) {
        [parts addObject:[NSString stringWithFormat:@"%@ posts", statuses]];
    }
    [parts addObject:[NSString stringWithFormat:@"%lu media items", (unsigned long)self.items.count]];
    if (self.planSummary.length > 0) {
        [parts addObject:self.planSummary];
    }
    self.statsLabel.stringValue = [parts componentsJoinedByString:@" · "];

    [self loadAvatar];
}

- (void)loadAvatar {
    NSString *urlString = self.accountInfo[@"profile_image"];
    if (![urlString isKindOfClass:NSString.class] || urlString.length == 0) {
        self.avatarView.hidden = YES;
        return;
    }
    // The API hands out the small `_normal` variant; swap in a sharper one.
    urlString = [urlString stringByReplacingOccurrencesOfString:@"_normal" withString:@"_400x400"];
    if ([urlString isEqualToString:self.avatarURL]) {
        self.avatarView.hidden = NO;
        return;
    }
    self.avatarURL = urlString;
    NSURL *url = [NSURL URLWithString:urlString];
    if (!url) {
        self.avatarView.hidden = YES;
        return;
    }
    __weak typeof(self) weakSelf = self;
    NSString *requestedURL = urlString;
    [[NSURLSession.sharedSession dataTaskWithURL:url
                               completionHandler:^(NSData *data, NSURLResponse *response, NSError *error) {
        if (!data || error) {
            return;
        }
        NSImage *image = [[NSImage alloc] initWithData:data];
        if (!image) {
            return;
        }
        dispatch_async(dispatch_get_main_queue(), ^{
            __strong typeof(weakSelf) self = weakSelf;
            // Ignore a stale response after the user fetched someone else.
            if (!self || ![self.avatarURL isEqualToString:requestedURL]) {
                return;
            }
            self.avatarView.image = image;
            self.avatarView.hidden = NO;
        });
    }] resume];
}

#pragma mark - Table context menu

- (NSInteger)menuRow {
    NSInteger row = self.tableView.clickedRow;
    if (row < 0) {
        row = self.tableView.selectedRow;
    }
    if (row < 0 || row >= (NSInteger)self.items.count) {
        return -1;
    }
    return row;
}

- (void)showItemInFinder:(id)sender {
    NSInteger row = [self menuRow];
    if (row < 0) {
        return;
    }
    NSString *path = self.itemPaths[@(row)];
    if (path.length == 0 || ![NSFileManager.defaultManager fileExistsAtPath:path]) {
        self.downloadStatusLabel.stringValue = @"Not downloaded yet — no file to show.";
        return;
    }
    [NSWorkspace.sharedWorkspace activateFileViewerSelectingURLs:@[ [NSURL fileURLWithPath:path] ]];
}

- (void)openItemTweet:(id)sender {
    NSInteger row = [self menuRow];
    if (row < 0) {
        return;
    }
    NSString *tweetID = [NSString stringWithFormat:@"%@", self.items[row][@"tweet_id"] ?: @""];
    if (tweetID.length == 0) {
        return;
    }
    NSURL *url = [NSURL URLWithString:[@"https://x.com/i/status/" stringByAppendingString:tweetID]];
    [NSWorkspace.sharedWorkspace openURL:url];
}

- (void)updateControlAvailability {
    BOOL ready = self.client.state == XDLCoreStateReady;
    self.fetchButton.enabled = ready && !self.fetching && !self.downloading;
    self.stopFetchButton.enabled = self.fetching;
    if (!ready) {
        self.fetchStatusLabel.stringValue = @"Starting core…";
    } else if (!self.fetching && [self.fetchStatusLabel.stringValue isEqualToString:@"Starting core…"]) {
        self.fetchStatusLabel.stringValue = @"";
    }
}

// A new fetch or a library load starts a fresh context — stale progress from
// the previous account's download must not linger.
- (void)resetDownloadProgress {
    self.downloadBar.maxValue = 1;
    self.downloadBar.doubleValue = 0;
    self.downloadStatusLabel.stringValue = @"";
}

- (void)updateResultControls {
    self.downloadButton.enabled = self.items.count > 0 && !self.downloading && !self.fetching &&
                                  self.client.state == XDLCoreStateReady;
    self.stopDownloadButton.enabled = self.downloading;
    self.openFolderButton.enabled = self.activeHandle.length > 0;
}

- (void)openDownloadFolder {
    __weak typeof(self) weakSelf = self;
    void (^open)(NSString *) = ^(NSString *basePath) {
        XDLSettings *settings = XDLSettings.shared;
        NSString *folder = settings.folderTemplate;
        folder = [folder stringByReplacingOccurrencesOfString:@"{handle}" withString:weakSelf.activeHandle ?: @""];
        folder = [folder stringByReplacingOccurrencesOfString:@"{name}"
                                                   withString:weakSelf.accountInfo[@"nick"] ?: weakSelf.activeHandle ?: @""];
        NSString *path = [basePath stringByAppendingPathComponent:folder];
        BOOL isDir = NO;
        if (![NSFileManager.defaultManager fileExistsAtPath:path isDirectory:&isDir] || !isDir) {
            path = basePath;
        }
        [NSWorkspace.sharedWorkspace openURL:[NSURL fileURLWithPath:path]];
    };

    NSString *basePath = XDLSettings.shared.downloadPath;
    if (basePath.length > 0) {
        open(basePath);
        return;
    }
    [self.client get:@"/defaults" completion:^(id json, NSError *error) {
        NSString *fallback = [json isKindOfClass:NSDictionary.class] ? json[@"download_path"] : nil;
        open([fallback isKindOfClass:NSString.class] ? fallback : NSHomeDirectory());
    }];
}

+ (void)presentError:(NSError *)error {
    [self presentMessage:error.localizedDescription];
}

+ (void)presentMessage:(NSString *)message {
    NSAlert *alert = [[NSAlert alloc] init];
    alert.messageText = @"XDL";
    alert.informativeText = message ?: @"";
    [alert runModal];
}

#pragma mark - Table

- (NSInteger)numberOfRowsInTableView:(NSTableView *)tableView {
    return self.items.count;
}

- (NSTableRowView *)tableView:(NSTableView *)tableView rowViewForRow:(NSInteger)row {
    return [[XDLTableRowView alloc] initWithFrame:NSZeroRect];
}

- (NSView *)tableView:(NSTableView *)tableView viewForTableColumn:(NSTableColumn *)tableColumn row:(NSInteger)row {
    NSString *identifier = tableColumn.identifier;
    NSDictionary *entry = self.items[row];

    XDLTableCell *cell = [tableView makeViewWithIdentifier:identifier owner:self];
    if (!cell) {
        cell = [[XDLTableCell alloc] initWithFrame:NSZeroRect];
        cell.identifier = identifier;
    }
    NSTextField *label = cell.label;
    label.font = [NSFont systemFontOfSize:12 weight:NSFontWeightRegular];
    label.textColor = XDLTheme.label;

    if ([identifier isEqualToString:@"type"]) {
        label.stringValue = [entry[@"type"] isKindOfClass:NSString.class] ? entry[@"type"] : @"";
    } else if ([identifier isEqualToString:@"date"]) {
        label.stringValue = [entry[@"date"] isKindOfClass:NSString.class] ? entry[@"date"] : @"";
    } else if ([identifier isEqualToString:@"tweet"]) {
        label.stringValue = [NSString stringWithFormat:@"%@", entry[@"tweet_id"] ?: @""];
        label.font = [NSFont monospacedDigitSystemFontOfSize:12 weight:NSFontWeightRegular];
    } else if ([identifier isEqualToString:@"file"]) {
        NSString *file = entry[@"original_filename"];
        if (![file isKindOfClass:NSString.class] || file.length == 0) {
            file = [[entry[@"url"] isKindOfClass:NSString.class] ? entry[@"url"] : @"" lastPathComponent];
        }
        label.stringValue = file;
        label.textColor = XDLTheme.secondaryLabel;
    } else if ([identifier isEqualToString:@"status"]) {
        NSString *status = self.itemStatuses[@(row)] ?: @"";
        label.stringValue = status;
        if ([status isEqualToString:@"success"] || [status isEqualToString:@"downloaded"]) {
            label.textColor = XDLTheme.statusGreen;
        } else if ([status isEqualToString:@"failed"]) {
            label.textColor = XDLTheme.statusRed;
        } else if ([status isEqualToString:@"skipped"]) {
            label.textColor = XDLTheme.statusOrange;
        } else {
            label.textColor = XDLTheme.secondaryLabel;
        }
    }
    return cell;
}

@end
