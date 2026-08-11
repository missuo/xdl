#import <Cocoa/Cocoa.h>
#import "XDLPageViewController.h"

NS_ASSUME_NONNULL_BEGIN

@class XDLCoreClient;

/// Main page: fetch a user's media timeline via xtractor, list the media,
/// download with live per-item status.
@interface XDLFetchViewController : XDLPageViewController

- (instancetype)initWithClient:(XDLCoreClient *)client;

/// Load a previously saved account (from the Library page) into the results view.
- (void)loadSavedAccount:(NSString *)username mediaType:(NSString *)mediaType;

/// Prefill the form from a saved account and start fetching immediately.
- (void)refetchAccount:(NSString *)username mediaType:(NSString *)mediaType;

@end

NS_ASSUME_NONNULL_END
