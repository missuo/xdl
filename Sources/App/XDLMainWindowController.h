#import <Cocoa/Cocoa.h>

NS_ASSUME_NONNULL_BEGIN

@class XDLCoreClient;

@interface XDLMainWindowController : NSWindowController

- (instancetype)initWithCoreClient:(XDLCoreClient *)client;

/// Switch to a sidebar page programmatically (e.g. "fetch").
- (void)showPage:(NSString *)pageID;

@end

NS_ASSUME_NONNULL_END
